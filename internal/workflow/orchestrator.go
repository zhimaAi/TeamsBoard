package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"goteams-client/internal/applog"
	"goteams-client/internal/capability"
	"goteams-client/internal/cloud"
	"goteams-client/internal/executor"
	builtinskills "goteams-client/internal/skills"
	"goteams-client/internal/storage/log"
)

// EventBroadcaster is the event broadcast callback type
type EventBroadcaster func(sessionUUID string, sequence int, event executor.ExecutorEvent)

// StateBroadcaster is the state-change broadcast callback type
type StateBroadcaster func(taskUUID, stepKey, sessionUUID, status string)

// RunStepOptions are the run-step options
type RunStepOptions struct {
	TaskUUID  string
	StepKey   string
	WorkDir   string
	RequestID string
	CLIType   string // CLI type chosen by the user (optional; empty falls back to the cloud snapshot)
	Model     string // Model chosen by the user (optional)
}

// ContinueConversationOptions are the options for continuing a historical CLI conversation.
type ContinueConversationOptions struct {
	ParentSessionUUID string
	Prompt            string
	RequestID         string
}

type activeSession struct {
	cancel   context.CancelFunc
	adapter  executor.Adapter
	stopping bool
}

// Orchestrator is the workflow orchestrator
type Orchestrator struct {
	db          *sql.DB
	logDB       *sql.DB
	eventStore  *log.EventStore
	wsClient    *cloud.WSClient
	cloudClient *cloud.Client

	mu             sync.RWMutex
	activeSessions map[string]*activeSession
	syncMu         sync.Mutex

	eventBroadcaster EventBroadcaster
	stateBroadcaster StateBroadcaster

	localAPIBaseURL string
	skillsRoot      string
	capabilities    *capability.Registry
}

// ConfigureSkillRuntime sets the runtime needed for a CLI Session to call local Skills.
func (o *Orchestrator) ConfigureSkillRuntime(baseURL, skillsRoot string, capabilities *capability.Registry) {
	o.localAPIBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	o.skillsRoot = skillsRoot
	o.capabilities = capabilities
}

// NewOrchestrator creates the orchestrator
func NewOrchestrator(db *sql.DB, logDB *sql.DB, eventStore *log.EventStore, wsClient *cloud.WSClient, cloudClient *cloud.Client) *Orchestrator {
	return &Orchestrator{
		db:             db,
		logDB:          logDB,
		eventStore:     eventStore,
		wsClient:       wsClient,
		cloudClient:    cloudClient,
		activeSessions: make(map[string]*activeSession),
	}
}

// SetEventBroadcaster sets the event broadcast callback
func (o *Orchestrator) SetEventBroadcaster(cb EventBroadcaster) {
	o.eventBroadcaster = cb
}

// SetStateBroadcaster sets the state-change broadcast callback
func (o *Orchestrator) SetStateBroadcaster(cb StateBroadcaster) {
	o.stateBroadcaster = cb
}

// RunStep executes a step
func (o *Orchestrator) RunStep(ctx context.Context, opts RunStepOptions) (string, error) {
	// 1. Pre-flight validation
	if err := o.validateRun(ctx, opts); err != nil {
		return "", err
	}

	// 2. Get step info
	var stepUUID, agentCLIType, stepCLIType, promptSnapshot string
	err := o.db.QueryRow(
		`SELECT s.uuid, t.cli_type, s.cli_type, s.prompt_snapshot
		 FROM gt_task_steps s
		 JOIN gt_tasks t ON t.uuid = s.task_uuid
		 WHERE s.task_uuid = ? AND s.step_key = ?`,
		opts.TaskUUID, opts.StepKey).Scan(&stepUUID, &agentCLIType, &stepCLIType, &promptSnapshot)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("步骤不存在: %s", opts.StepKey)
	}
	if err != nil {
		return "", fmt.Errorf("查询步骤失败: %w", err)
	}

	// 3. Determine CLI type: prefer the user's choice, otherwise fall back to the cloud Agent snapshot.
	cliType := opts.CLIType
	if cliType == "" {
		cliType = agentCLIType
	}
	if cliType == "" {
		cliType = stepCLIType
	}
	execPath, err := resolveCLIExecutable(cliType)
	if err != nil {
		return "", err
	}

	// 4. Create conversation_uuid + session_uuid and generate a continuous round number per step.
	// Each re-execution of a step creates a new conversation_uuid, but in the UI and cloud projection
	// it is still the next round of the step, so run_no must not be reset to 1 for a new conversation.
	conversationUUID := uuid.New().String()
	sessionUUID := uuid.New().String()
	now := nowMillis()
	var runNo int
	if err := o.db.QueryRow(
		`SELECT COALESCE(MAX(run_no), 0) + 1 FROM gt_cli_sessions WHERE step_uuid = ?`,
		stepUUID,
	).Scan(&runNo); err != nil {
		return "", fmt.Errorf("读取步骤轮次失败: %w", err)
	}

	// 5. Write gt_cli_sessions
	_, err = o.db.Exec(
		`INSERT INTO gt_cli_sessions (uuid, task_uuid, step_uuid, conversation_uuid, run_no, status, cli_type, work_dir, prompt_snapshot, prompt_hash, started_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionUUID, opts.TaskUUID, stepUUID, conversationUUID, runNo,
		SessionStatusCreated, cliType, opts.WorkDir, promptSnapshot, sha256Hex(promptSnapshot), now, now, now)
	if err != nil {
		return "", fmt.Errorf("创建 Session 失败: %w", err)
	}

	// 6. Update step and task status to running
	o.db.Exec(`UPDATE gt_task_steps SET execution_status = 'running', updated_at = ? WHERE uuid = ?`, now, stepUUID)
	o.db.Exec(`UPDATE gt_tasks SET execution_status = 'running', updated_at = ? WHERE uuid = ?`, now, opts.TaskUUID)

	// Broadcast the state change
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(opts.TaskUUID, opts.StepKey, sessionUUID, SessionStatusRunning)
	}

	// Task starts executing (-> running): sync to the cloud immediately
	o.SyncTaskNow(opts.TaskUUID)

	// 7. Start the CLI process asynchronously
	go o.runCLIProcess(context.WithoutCancel(ctx), sessionUUID, opts, cliType, execPath, promptSnapshot, false, "", opts.Model)

	return sessionUUID, nil
}

// ContinueConversation creates a new local Session on a historical CLI native session.
func (o *Orchestrator) ContinueConversation(ctx context.Context, opts ContinueConversationOptions) (string, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("问题内容不能为空")
	}

	var taskUUID, stepUUID, stepKey, conversationUUID, cliType, workDir, externalSessionID, parentStatus string
	err := o.db.QueryRow(
		`SELECT s.task_uuid, s.step_uuid, ts.step_key, s.conversation_uuid, s.cli_type, s.work_dir, s.external_session_id, s.status
		 FROM gt_cli_sessions s
		 JOIN gt_task_steps ts ON ts.uuid = s.step_uuid
		 WHERE s.uuid = ?`,
		opts.ParentSessionUUID,
	).Scan(&taskUUID, &stepUUID, &stepKey, &conversationUUID, &cliType, &workDir, &externalSessionID, &parentStatus)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("历史会话不存在")
	}
	if err != nil {
		return "", fmt.Errorf("读取历史会话失败: %w", err)
	}
	if !IsTerminalStatus(parentStatus) {
		return "", fmt.Errorf("当前一轮仍在执行，请等待完成或先终止")
	}

	if externalSessionID == "" {
		_ = o.db.QueryRow(
			`SELECT external_session_id FROM gt_cli_sessions
			 WHERE conversation_uuid = ? AND external_session_id <> ''
			 ORDER BY run_no DESC LIMIT 1`,
			conversationUUID,
		).Scan(&externalSessionID)
	}
	if externalSessionID == "" {
		return "", fmt.Errorf("CLI 未返回可继续的会话 ID，请重新执行步骤创建新对话")
	}

	if err := o.validateRun(ctx, RunStepOptions{TaskUUID: taskUUID, StepKey: stepKey, WorkDir: workDir}); err != nil {
		return "", err
	}
	execPath, err := resolveCLIExecutable(cliType)
	if err != nil {
		return "", err
	}

	var runNo int
	if err := o.db.QueryRow(
		`SELECT COALESCE(MAX(run_no), 0) + 1 FROM gt_cli_sessions WHERE step_uuid = ?`,
		stepUUID,
	).Scan(&runNo); err != nil {
		return "", fmt.Errorf("读取步骤轮次失败: %w", err)
	}

	sessionUUID := uuid.New().String()
	now := nowMillis()
	_, err = o.db.Exec(
		`INSERT INTO gt_cli_sessions
		 (uuid, task_uuid, step_uuid, conversation_uuid, parent_session_uuid, run_no, status, cli_type, external_session_id, work_dir, prompt_snapshot, prompt_hash, started_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionUUID, taskUUID, stepUUID, conversationUUID, opts.ParentSessionUUID, runNo,
		SessionStatusCreated, cliType, externalSessionID, workDir, prompt, sha256Hex(prompt), now, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("创建继续对话 Session 失败: %w", err)
	}

	o.db.Exec(`UPDATE gt_task_steps SET execution_status = 'running', updated_at = ? WHERE uuid = ?`, now, stepUUID)
	o.db.Exec(`UPDATE gt_tasks SET execution_status = 'running', updated_at = ? WHERE uuid = ?`, now, taskUUID)
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, stepKey, sessionUUID, SessionStatusRunning)
	}

	// Task continues conversation / re-enters execution (-> running): sync to the cloud immediately
	o.SyncTaskNow(taskUUID)

	runOpts := RunStepOptions{
		TaskUUID:  taskUUID,
		StepKey:   stepKey,
		WorkDir:   workDir,
		RequestID: opts.RequestID,
	}
	go o.runCLIProcess(context.WithoutCancel(ctx), sessionUUID, runOpts, cliType, execPath, prompt, true, externalSessionID, "")
	return sessionUUID, nil
}

// runCLIProcess runs the CLI process asynchronously
func (o *Orchestrator) runCLIProcess(
	ctx context.Context,
	sessionUUID string,
	opts RunStepOptions,
	cliType, execPath, prompt string,
	resume bool,
	externalSessionID string,
	model string,
) {
	// Create a cancellable context
	innerCtx, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	o.activeSessions[sessionUUID] = &activeSession{cancel: cancel}
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.activeSessions, sessionUUID)
		o.mu.Unlock()
	}()

	var initialStopRequestedAt int64
	_ = o.db.QueryRow(`SELECT stop_requested_at FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&initialStopRequestedAt)
	if initialStopRequestedAt > 0 {
		o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, SessionStatusStopped, externalSessionID, 0, 0, "")
		return
	}

	prompt = o.resolveLocalRuntimePlaceholders(prompt)
	skillToken, skillEnv, err := o.prepareSkillRuntime(prompt, opts)
	if err != nil {
		applog.Warn("[Orchestrator] 准备 Skill 运行环境失败", "error", err)
		o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, SessionStatusFailed, externalSessionID, 0, 0, "")
		return
	}
	if skillToken != "" {
		defer o.capabilities.Revoke(skillToken)
	}

	// Create the adapter
	var adapter executor.Adapter
	switch cliType {
	case executor.CLITypeCodex:
		adapter = codexNewAdapter()
	case executor.CLITypeClaude:
		adapter = claudeNewAdapter()
	case executor.CLITypeCodeBuddy:
		adapter = codeBuddyNewAdapter()
	case executor.CLITypeOpenCode:
		adapter = openCodeNewAdapter()
	default:
		o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, SessionStatusFailed, "", 0, 0, "")
		return
	}
	o.mu.Lock()
	shouldStopBeforeStart := false
	if active := o.activeSessions[sessionUUID]; active != nil {
		if active.stopping {
			shouldStopBeforeStart = true
		} else {
			active.adapter = adapter
		}
	}
	o.mu.Unlock()
	if shouldStopBeforeStart {
		cancel()
		o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, SessionStatusStopped, externalSessionID, 0, 0, "")
		return
	}

	// Run
	runOptions := executor.RunOptions{
		ExecPath:     execPath,
		Prompt:       prompt,
		WorkDir:      opts.WorkDir,
		ModelProfile: model,
		EnvVars:      skillEnv,
	}
	eventCh, err := startAdapterConversation(innerCtx, adapter, runOptions, resume, externalSessionID)
	if err != nil {
		applog.Warn("[Orchestrator] 启动 CLI 失败", "error", err)
		o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, SessionStatusFailed, "", 0, 0, "")
		return
	}
	o.db.Exec(
		`UPDATE gt_cli_sessions SET status = ?, updated_at = ? WHERE uuid = ? AND status = ?`,
		SessionStatusRunning, nowMillis(), sessionUUID, SessionStatusCreated,
	)

	// Handle events
	sessionID := ""
	// Token accounting uses the adapter's final complete event (each adapter has already summed this execution internally).
	// Mid-way usage events are only a fallback: used only when the adapter aborts abnormally and no complete is available,
	// so they are accumulated separately in usage* variables to avoid double-counting against the complete total.
	usageInputTokens := 0
	usageOutputTokens := 0
	inputTokens := 0
	outputTokens := 0
	completeTokensSeen := false
	responseContent := ""

	for event := range eventCh {
		// After termination, the adapter may still return the native session ID in the final complete/error event.
		// Even when stopping persistence and broadcast of other events, keep that ID first so the user can continue from a terminated round.
		if event.SessionID != "" {
			sessionID = event.SessionID
			o.db.Exec(
				`UPDATE gt_cli_sessions SET external_session_id = ?, updated_at = ? WHERE uuid = ?`,
				sessionID, nowMillis(), sessionUUID,
			)
		}
		eventError := strings.TrimSpace(event.Error)
		if eventError != "" {
			o.db.Exec(
				`UPDATE gt_cli_sessions SET error_message = ?, updated_at = ? WHERE uuid = ?`,
				truncate(eventError, 8192), nowMillis(), sessionUUID,
			)
		}

		o.mu.RLock()
		active := o.activeSessions[sessionUUID]
		stopping := active != nil && active.stopping
		o.mu.RUnlock()
		if stopping {
			// After the stop request is sent, keep draining the adapter channel but no longer persist or broadcast
			// the buffered Claude/Codex events, to avoid the UI continuing to flash messages after "terminating".
			if eventError != "" {
				o.recomputeAndSyncSession(opts.TaskUUID, opts.StepKey)
			}
			continue
		}

		// Write to the log DB
		seq, _ := o.eventStore.NextSequence(sessionUUID)
		o.eventStore.AppendFromJSON(sessionUUID, seq, event.Type, event)

		// Broadcast events
		if o.eventBroadcaster != nil {
			o.eventBroadcaster(sessionUUID, seq, event)
		}

		// Extract token: mid-way events only accumulate into the fallback variable
		if event.Type != executor.EventComplete {
			if event.InputTokens > 0 {
				usageInputTokens += event.InputTokens
			}
			if event.OutputTokens > 0 {
				usageOutputTokens += event.OutputTokens
			}
		}
		// Each AI-visible message is already persisted as an independent event and uploaded one by one in the cloud snapshot.
		// response_summary keeps only the last one, as a compatibility field for the old cloud.
		if event.Type == executor.EventMessage && strings.TrimSpace(event.Content) != "" {
			responseContent = strings.TrimSpace(event.Content)
		}

		// Completion event
		if event.Type == executor.EventComplete {
			if event.SessionID != "" {
				sessionID = event.SessionID
			}
			// complete carries the total of this execution; overwrite rather than accumulate
			inputTokens = event.InputTokens
			outputTokens = event.OutputTokens
			completeTokensSeen = true
			break
		}
		if event.Type == executor.EventError {
			status := SessionStatusFailed
			var stopRequestedAt int64
			_ = o.db.QueryRow(`SELECT stop_requested_at FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&stopRequestedAt)
			if stopRequestedAt > 0 {
				status = SessionStatusStopped
			}
			// Abnormal termination cannot get complete; fall back to mid-way usage accumulation
			o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, status, sessionID, usageInputTokens, usageOutputTokens, responseContent)
			return
		}
	}

	// When the channel is closed but no complete arrives (process killed, etc.), also fall back to usage accumulation
	if !completeTokensSeen {
		inputTokens = usageInputTokens
		outputTokens = usageOutputTokens
	}

	status := SessionStatusSuccess
	var stopRequestedAt int64
	_ = o.db.QueryRow(`SELECT stop_requested_at FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&stopRequestedAt)
	if stopRequestedAt > 0 {
		status = SessionStatusStopped
	}
	o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, status, sessionID, inputTokens, outputTokens, responseContent)
}

// startAdapterConversation explicitly chooses new or resume conversation by call semantics.
//
// Resuming can no longer rely on whether externalSessionID is empty to implicitly downgrade to a new conversation; once the caller declares resume
// it must fail outright if the native session ID is missing, to avoid Codex/Claude/CodeBuddy silently creating a new session.
func startAdapterConversation(
	ctx context.Context,
	adapter executor.Adapter,
	runOptions executor.RunOptions,
	resume bool,
	externalSessionID string,
) (<-chan executor.ExecutorEvent, error) {
	if !resume {
		return adapter.NewConversation(ctx, runOptions)
	}

	externalSessionID = strings.TrimSpace(externalSessionID)
	if externalSessionID == "" {
		return nil, fmt.Errorf("恢复 CLI 对话失败：原生会话 ID 为空")
	}
	return adapter.ResumeConversation(ctx, executor.ResumeOptions{
		RunOptions:        runOptions,
		ExternalSessionID: externalSessionID,
	})
}

// latestVisibleResponseContent reads the last AI-visible reply of this round from the local event log.
// On manual termination the execution goroutine may not reach finishSession in time, so the in-memory responseContent alone cannot be relied upon.
func (o *Orchestrator) latestVisibleResponseContent(sessionUUID string) (string, error) {
	if o.logDB == nil {
		return "", nil
	}

	var response sql.NullString
	err := o.logDB.QueryRow(
		`SELECT trim(json_extract(e.payload_json, '$.content'))
		 FROM gt_session_events e
		 WHERE e.session_uuid = ?
		   AND e.event_type = ?
		   AND json_valid(e.payload_json)
		   AND trim(COALESCE(json_extract(e.payload_json, '$.content'), '')) <> ''
		 ORDER BY e.sequence DESC
		 LIMIT 1`,
		sessionUUID, executor.EventMessage,
	).Scan(&response)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 Session 最终可见回复失败: %w", err)
	}
	return response.String, nil
}

// ensureSessionSummary idempotently writes the session summary needed for the cloud snapshot.
func (o *Orchestrator) ensureSessionSummary(sessionUUID, stepKey, responseContent string, inputTokens, outputTokens int, createdAt int64) error {
	_, err := o.db.Exec(
		`INSERT INTO gt_conversation_summaries
		 (uuid, conversation_uuid, session_uuid, step_key, round_no, request_summary, response_summary,
		  token_input, token_output, token_total, duration_ms, created_at)
		 SELECT ?, s.conversation_uuid, s.uuid, ?, s.run_no, substr(s.prompt_snapshot, 1, 8192), ?,
		        ?, ?, ?, s.duration_ms, ?
		 FROM gt_cli_sessions s
		 WHERE s.uuid = ?
		   AND NOT EXISTS (
		       SELECT 1 FROM gt_conversation_summaries cs WHERE cs.session_uuid = s.uuid
		   )`,
		uuid.New().String(), stepKey, truncate(responseContent, 16*1024),
		inputTokens, outputTokens, inputTokens+outputTokens, createdAt, sessionUUID,
	)
	return err
}

// finishSession completes the Session and updates its status
func (o *Orchestrator) finishSession(sessionUUID, taskUUID, stepKey, status, externalSessionID string, inputTokens, outputTokens int, responseContent string) {
	now := nowMillis()

	// Conditional update (avoid race: only non-terminal states can be updated)
	result, err := o.db.Exec(
		`UPDATE gt_cli_sessions
		 SET status = ?,
		     external_session_id = CASE WHEN ? <> '' THEN ? ELSE external_session_id END,
		     input_tokens = ?, output_tokens = ?, total_tokens = ?,
		     finished_at = ?, duration_ms = ? - started_at, updated_at = ?
		 WHERE uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		status, externalSessionID, externalSessionID, inputTokens, outputTokens, inputTokens+outputTokens,
		now, now, now, sessionUUID)
	if err != nil {
		applog.Warn("[Orchestrator] 更新 Session 状态失败", "error", err)
		return
	}

	rows, err := result.RowsAffected()
	if err != nil {
		applog.Warn("[Orchestrator] 获取影响行数失败", "error", err)
	} else if rows == 0 {
		// Already terminal; do not overwrite
		return
	}

	// Write summary
	if err := o.ensureSessionSummary(sessionUUID, stepKey, responseContent, inputTokens, outputTokens, now); err != nil {
		applog.Warn("[Orchestrator] 写入会话摘要失败", "error", err)
	}

	// Aggregate Step/Task status
	tx, err := o.db.Begin()
	if err == nil {
		if err := RecomputeStepStatus(tx, taskUUID, stepKey); err != nil {
			applog.Warn("[Orchestrator] 重算 Step 状态失败", "error", err)
		}
		if err := RecomputeTaskStatus(tx, taskUUID); err != nil {
			applog.Warn("[Orchestrator] 重算 Task 状态失败", "error", err)
		}
		if err := tx.Commit(); err != nil {
			applog.Warn("[Orchestrator] 提交状态重算事务失败", "error", err)
		}
	}

	// Unified task-change sync: mark dirty + push immediately when connected
	o.SyncTaskNow(taskUUID)

	// Broadcast the state change
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, stepKey, sessionUUID, status)
	}
}

// StopSession stops the session
func (o *Orchestrator) StopSession(ctx context.Context, sessionUUID string) error {
	var status, taskUUID, stepKey string
	err := o.db.QueryRow(
		`SELECT s.status, s.task_uuid, ts.step_key
		 FROM gt_cli_sessions s
		 JOIN gt_task_steps ts ON ts.uuid = s.step_uuid
		 WHERE s.uuid = ?`,
		sessionUUID,
	).Scan(&status, &taskUUID, &stepKey)
	if err != nil {
		return fmt.Errorf("查询 Session 失败: %w", err)
	}
	if IsTerminalStatus(status) {
		return nil // Already terminal
	}

	now := nowMillis()

	// First enter "terminating", but must not be written as stopped prematurely. Only after the process tree confirms termination,
	// is the final state committed; otherwise the UI would again show the false "terminated but still running in background" state.
	result, err := o.db.ExecContext(
		ctx,
		`UPDATE gt_cli_sessions
		 SET status = ?, stop_requested_at = ?, updated_at = ?
		 WHERE uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		SessionStatusStopRequested, now, now, sessionUUID,
	)
	if err != nil {
		return fmt.Errorf("记录停止请求失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取停止请求结果失败: %w", err)
	}
	if affected == 0 {
		return nil
	}

	// Set stopping before any potentially blocking process operation, to immediately stop event persistence and broadcast.
	o.mu.Lock()
	active := o.activeSessions[sessionUUID]
	var cancel context.CancelFunc
	var adapter executor.Adapter
	if active != nil {
		active.stopping = true
		cancel = active.cancel
		adapter = active.adapter
	}
	o.mu.Unlock()

	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, stepKey, sessionUUID, SessionStatusStopRequested)
	}

	// adapter.Stop must first terminate the entire process tree by root PID; only after success is the outer context cancelled.
	// If the process tree termination fails, keep the root process and the stop_requested state to let the user retry,
	// and return an explicit error rather than falsely reporting "terminated".
	if adapter != nil {
		if err := adapter.Stop(); err != nil {
			return fmt.Errorf("终止 CLI 进程树失败: %w", err)
		}
	}
	if cancel != nil {
		cancel()
	}

	// After the process tree terminates successfully, commit Session/Step/Task together into the stopped terminal state.
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启停止事务失败: %w", err)
	}
	defer tx.Rollback()

	result, err = tx.Exec(
		`UPDATE gt_cli_sessions
		 SET status = ?, finished_at = ?,
		     duration_ms = CASE WHEN started_at > 0 THEN ? - started_at ELSE 0 END,
		     error_message = CASE WHEN error_message <> '' THEN error_message ELSE ? END,
		     updated_at = ?
		 WHERE uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		SessionStatusStopped, now, now, "用户手动终止", now, sessionUUID,
	)
	if err != nil {
		return fmt.Errorf("更新停止状态失败: %w", err)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取停止结果失败: %w", err)
	}
	if affected > 0 {
		if err := RecomputeStepStatus(tx, taskUUID, stepKey); err != nil {
			return err
		}
		if err := RecomputeTaskStatus(tx, taskUUID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交停止状态失败: %w", err)
	}

	// Manual termination commits stopped before the execution goroutine's finishSession; before cloud sync,
	// the summary must be rebuilt from persisted message events and written, to avoid losing replies already shown before termination.
	responseContent, responseErr := o.latestVisibleResponseContent(sessionUUID)
	if responseErr != nil {
		applog.Warn("[Orchestrator] 重建已终止 Session 可见回复失败", "error", responseErr)
	}
	if err := o.ensureSessionSummary(sessionUUID, stepKey, responseContent, 0, 0, now); err != nil {
		applog.Warn("[Orchestrator] 写入已终止 Session 摘要失败", "error", err)
	}

	// Sync and broadcast only after the terminal state is committed, so client refresh and cloud refresh see a consistent stopped.
	if taskUUID != "" {
		o.SyncTaskNow(taskUUID)
	}
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, stepKey, sessionUUID, SessionStatusStopped)
	}

	return nil
}

func (o *Orchestrator) resolveLocalRuntimePlaceholders(prompt string) string {
	skillsRoot := o.skillsRoot
	if skillsRoot == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return prompt
		}
		skillsRoot = filepath.Join(homeDir, ".goteams", "skills")
	}
	runtimeCtx := RuntimeContext{
		APIBaseURL: o.localAPIBaseURL,
		SkillPaths: BuiltinSkillPaths(skillsRoot),
	}
	if len(ReferencedOperationalSkills(prompt)) > 0 {
		runtimeCtx.CapabilityToken = "injected-via-environment"
	}
	return ResolveRuntime(prompt, runtimeCtx)
}

func (o *Orchestrator) prepareSkillRuntime(prompt string, opts RunStepOptions) (string, map[string]string, error) {
	referenceText := prompt
	var storedPrompt string
	if err := o.db.QueryRow(
		`SELECT prompt_snapshot FROM gt_task_steps WHERE task_uuid = ? AND step_key = ?`,
		opts.TaskUUID, opts.StepKey,
	).Scan(&storedPrompt); err == nil {
		referenceText += "\n" + storedPrompt
	}
	referenced := ReferencedOperationalSkills(referenceText)
	if len(referenced) == 0 {
		return "", nil, nil
	}
	if o.capabilities == nil || o.localAPIBaseURL == "" {
		return "", nil, fmt.Errorf("本地 Skill 请求地址或能力令牌注册表未初始化")
	}

	skillsRoot := o.skillsRoot
	if skillsRoot == "" {
		return "", nil, fmt.Errorf("Skill 根目录未配置")
	}
	grant := capability.Grant{Skills: make(map[string]bool, len(referenced))}
	for _, name := range referenced {
		if err := builtinskills.Require(o.db, name); err != nil {
			return "", nil, err
		}
		skillFile := filepath.Join(skillsRoot, name, "SKILL.md")
		if info, err := os.Stat(skillFile); err != nil || info.IsDir() {
			return "", nil, fmt.Errorf("Skill %s 不可用: %s", name, skillFile)
		}
		grant.Skills[name] = true
	}

	if grant.Allows(builtinskills.SkillAPI) {
		if err := o.db.QueryRow(
			`SELECT api_collection_id, api_folder_id FROM gt_tasks WHERE uuid = ?`,
			opts.TaskUUID,
		).Scan(&grant.APICollectionID, &grant.APIFolderID); err != nil {
			return "", nil, fmt.Errorf("读取任务接口范围失败: %w", err)
		}
		if grant.APICollectionID <= 0 || grant.APIFolderID <= 0 {
			return "", nil, fmt.Errorf("任务未配置有效的接口集合和文件夹")
		}
	}

	token, err := o.capabilities.Issue(grant)
	if err != nil {
		return "", nil, fmt.Errorf("签发 Skill 能力令牌失败: %w", err)
	}
	env := map[string]string{
		"GOTEAMS_LOCAL_BASE_URL":         o.localAPIBaseURL,
		"GOTEAMS_LOCAL_CAPABILITY_TOKEN": token,
		"PYTHONIOENCODING":               "utf-8",
	}
	if grant.APICollectionID > 0 {
		env["GOTEAMS_API_COLLECTION_ID"] = fmt.Sprintf("%d", grant.APICollectionID)
		env["GOTEAMS_API_FOLDER_ID"] = fmt.Sprintf("%d", grant.APIFolderID)
	}
	return token, env, nil
}

// validateRun pre-flight validation
func (o *Orchestrator) validateRun(ctx context.Context, opts RunStepOptions) error {
	// Check task exists
	var taskStatus string
	err := o.db.QueryRow(`SELECT status FROM gt_tasks WHERE uuid = ?`, opts.TaskUUID).Scan(&taskStatus)
	if err == sql.ErrNoRows {
		return fmt.Errorf("任务不存在: %s", opts.TaskUUID)
	}
	if err != nil {
		return fmt.Errorf("查询任务失败: %w", err)
	}

	// Check concurrency limit (at most 1 active Session per task at a time)
	var activeCount int
	o.db.QueryRow(
		`SELECT COUNT(*) FROM gt_cli_sessions WHERE task_uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		opts.TaskUUID).Scan(&activeCount)
	if activeCount > 0 {
		return fmt.Errorf("任务已有活动 Session")
	}

	// Check working directory
	if opts.WorkDir == "" {
		return fmt.Errorf("任务未配置工作目录")
	}
	info, err := os.Stat(opts.WorkDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("任务工作目录不存在: %s", opts.WorkDir)
		}
		return fmt.Errorf("无法访问任务工作目录 %s: %w", opts.WorkDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("任务工作目录不是文件夹: %s", opts.WorkDir)
	}

	return nil
}

// GlobalRunningSessionCount returns the global (cross-task) count of running CLI sessions,
// used to show the live count of in-progress CLI tasks in the bottom-left badge.
func (o *Orchestrator) GlobalRunningSessionCount() (int, error) {
	var count int
	err := o.db.QueryRow(
		`SELECT COUNT(*) FROM gt_cli_sessions WHERE status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计运行中的 CLI 会话失败: %w", err)
	}
	return count, nil
}

// resolveCLIExecutable locates the executable from the system PATH by CLI type.
func resolveCLIExecutable(cliType string) (string, error) {
	executableNames := []string{cliType}
	switch cliType {
	case executor.CLITypeCodex, executor.CLITypeClaude, executor.CLITypeCodeBuddy, executor.CLITypeOpenCode:
	default:
		return "", fmt.Errorf("CLI 类型无效: %q", cliType)
	}
	if cliType == executor.CLITypeCodeBuddy {
		executableNames = append(executableNames, "cbc")
	}

	var lookupErr error
	for _, executableName := range executableNames {
		execPath, err := osexec.LookPath(executableName)
		if err == nil {
			return execPath, nil
		}
		lookupErr = err
	}
	if cliType == executor.CLITypeCodeBuddy {
		return "", fmt.Errorf("未找到 CodeBuddy CLI（codebuddy 或 cbc），请先安装并加入 PATH: %w", lookupErr)
	}
	return "", fmt.Errorf("未找到 %s CLI，请先安装并加入 PATH: %w", cliType, lookupErr)
}

// codexNewAdapter creates the Codex adapter
func codexNewAdapter() executor.Adapter {
	return codexAdapterFactory()
}

// claudeNewAdapter creates the Claude adapter
func claudeNewAdapter() executor.Adapter {
	return claudeAdapterFactory()
}

// codeBuddyNewAdapter creates the CodeBuddy adapter
func codeBuddyNewAdapter() executor.Adapter {
	return codeBuddyAdapterFactory()
}

// openCodeNewAdapter creates the OpenCode adapter
func openCodeNewAdapter() executor.Adapter {
	return openCodeAdapterFactory()
}

// Adapter factories (injected from concrete subpackages in separate files to avoid import cycles)
var codexAdapterFactory = func() executor.Adapter { return nil }
var claudeAdapterFactory = func() executor.Adapter { return nil }
var codeBuddyAdapterFactory = func() executor.Adapter { return nil }
var openCodeAdapterFactory = func() executor.Adapter { return nil }

// SetCodexAdapterFactory sets the Codex adapter factory
func SetCodexAdapterFactory(factory func() executor.Adapter) {
	codexAdapterFactory = factory
}

// SetClaudeAdapterFactory sets the Claude adapter factory
func SetClaudeAdapterFactory(factory func() executor.Adapter) {
	claudeAdapterFactory = factory
}

// SetCodeBuddyAdapterFactory sets the CodeBuddy adapter factory
func SetCodeBuddyAdapterFactory(factory func() executor.Adapter) {
	codeBuddyAdapterFactory = factory
}

// SetOpenCodeAdapterFactory sets the OpenCode adapter factory
func SetOpenCodeAdapterFactory(factory func() executor.Adapter) {
	openCodeAdapterFactory = factory
}
