package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"

	"goteams-client/internal/applog"
	"goteams-client/internal/executor"
	"goteams-client/internal/storage/log"
)

// EventBroadcaster is the event broadcast callback type
type EventBroadcaster func(sessionUUID string, sequence int, event executor.ExecutorEvent)

// latestEventPersistIntervalMs is the throttling interval for persisting the
// latest CLI activity to gt_cli_sessions during event processing.
const latestEventPersistIntervalMs int64 = 1000

// 执行过程事件（gt_session_events）的内容截断上限：
// thinking 是完整思考文本可较长；工具调用/结果、中间输出与权限请求只保留摘要即可支撑动态区展示。
const (
	sessionEventThinkingLimit = 8192
	sessionEventToolLimit     = 4096
)

// sessionEventContentLimit 判定事件是否留存到执行过程回放表，并给出内容截断上限。
// 只留存过程类事件：思考、中间输出、工具调用、工具结果、权限请求；
// start / usage / complete 等生命周期事件由会话状态与 token 字段承载，不重复落库。
func sessionEventContentLimit(eventType string) (int, bool) {
	switch eventType {
	case executor.EventThinking:
		return sessionEventThinkingLimit, true
	case executor.EventMessage, executor.EventToolCall, executor.EventToolResult, executor.EventPermission:
		return sessionEventToolLimit, true
	default:
		return 0, false
	}
}

// StateBroadcaster is the state-change broadcast callback type
type StateBroadcaster func(taskUUID, stepKey, sessionUUID, status string)

// ActivityBroadcaster pushes the latest CLI activity (throttled) to task views,
// so running-step subtitles stay fresh without polling /progress.
type ActivityBroadcaster func(taskUUID, sessionUUID, eventType, content string, at int64)

// RunStepOptions are the run-step options
type RunStepOptions struct {
	TaskUUID  string
	StepUUID  string
	StepKey   string // compatibility lookup; new callers should use StepUUID
	WorkDir   string
	RequestID string
	CLIType   string // optional compatibility override
	Model     string // optional compatibility override
	// UserPrompt is the complete prompt passed into the task prompt builder.
	UserPrompt string
	// DisplayPrompt is persisted for the user-message record. When omitted,
	// UserPrompt remains the backwards-compatible display value.
	DisplayPrompt      string
	RecordType         string // initial_run | user_question
	ExecutionMode      string
	ExpertChainUUID    string
	ExpertChainRound   int
	TriggerSessionUUID string
}

// ContinueConversationOptions are the options for continuing a historical CLI conversation.
type ContinueConversationOptions struct {
	ParentSessionUUID string
	// Prompt is the complete prompt passed to the CLI. It can contain local
	// attachment paths, which are an execution detail rather than user-facing
	// conversation content.
	Prompt string
	// DisplayPrompt is persisted for the user-message record. When omitted by
	// an older caller, Prompt remains the backwards-compatible fallback.
	DisplayPrompt      string
	RequestID          string
	CLIType            string
	Model              string
	InternalHandoff    bool
	ExpertChainUUID    string
	ExpertChainRound   int
	TriggerSessionUUID string
}

type activeSession struct {
	cancel   context.CancelFunc
	adapter  executor.Adapter
	stopping bool
}

// Orchestrator is the workflow orchestrator
type Orchestrator struct {
	db         *sql.DB
	logDB      *sql.DB
	eventStore *log.EventStore

	mu             sync.RWMutex
	activeSessions map[string]*activeSession
	executionGates *taskExecutionGateRegistry

	eventBroadcaster    EventBroadcaster
	stateBroadcaster    StateBroadcaster
	stateListeners      []StateBroadcaster
	activityBroadcaster ActivityBroadcaster
}

// NewOrchestrator creates the orchestrator
func NewOrchestrator(db *sql.DB, logDB *sql.DB, eventStore *log.EventStore) *Orchestrator {
	return &Orchestrator{
		db:             db,
		logDB:          logDB,
		eventStore:     eventStore,
		activeSessions: make(map[string]*activeSession),
		executionGates: newTaskExecutionGateRegistry(),
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

// SetActivityBroadcaster sets the latest-activity broadcast callback
func (o *Orchestrator) SetActivityBroadcaster(cb ActivityBroadcaster) {
	o.activityBroadcaster = cb
}

// AddStateListener appends an extra observer of session state changes (e.g. the
// cloud task sync push). Listeners run after the main broadcaster and must not block.
func (o *Orchestrator) AddStateListener(cb StateBroadcaster) {
	if cb == nil {
		return
	}
	o.stateListeners = append(o.stateListeners, cb)
}

// RunStep executes a step
func (o *Orchestrator) RunStep(ctx context.Context, opts RunStepOptions) (string, error) {
	release := o.executionGates.acquire(opts.TaskUUID, false)
	defer release()
	return o.runStep(ctx, opts)
}

func (o *Orchestrator) runStep(ctx context.Context, opts RunStepOptions) (string, error) {
	// 1. Resolve the immutable task-step snapshot.
	var stepUUID, stepKey, stepCLIType, modelName, workDir, snapshotUUID, expertSnapshotUUID, executionMode string
	err := o.db.QueryRowContext(ctx,
		`SELECT s.uuid, s.step_key, s.cli_type, s.model_name, t.work_dir, s.task_pipeline_snapshot_uuid,
		        s.task_expert_group_snapshot_uuid, COALESCE(NULLIF(s.execution_mode,''), t.execution_mode)
		 FROM gt_task_steps s JOIN gt_tasks t ON t.uuid = s.task_uuid
		 WHERE s.task_uuid = ? AND ((? <> '' AND s.uuid = ?) OR (? = '' AND s.step_key = ?))`,
		opts.TaskUUID, opts.StepUUID, opts.StepUUID, opts.StepUUID, opts.StepKey).Scan(
		&stepUUID, &stepKey, &stepCLIType, &modelName, &workDir, &snapshotUUID, &expertSnapshotUUID, &executionMode)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("Agent 编排不存在")
	}
	if err != nil {
		return "", fmt.Errorf("查询步骤失败: %w", err)
	}

	// 3. Determine CLI type: prefer the user's choice, otherwise fall back to the cloud Agent snapshot.
	cliType := strings.TrimSpace(opts.CLIType)
	if cliType == "" {
		cliType = stepCLIType
	}
	if strings.TrimSpace(opts.Model) != "" {
		modelName = strings.TrimSpace(opts.Model)
	}
	if strings.TrimSpace(opts.WorkDir) == "" {
		opts.WorkDir = workDir
	}
	opts.StepUUID = stepUUID
	opts.StepKey = stepKey
	if err := o.validateRun(ctx, opts); err != nil {
		return "", err
	}
	displayPrompt := strings.TrimSpace(opts.DisplayPrompt)
	if displayPrompt == "" {
		displayPrompt = strings.TrimSpace(opts.UserPrompt)
	}
	var promptSnapshot string
	promptSnapshot, err = BuildRoundPrompt(ctx, o.db, executionMode, opts.TaskUUID, stepUUID, opts.UserPrompt)
	if err != nil {
		return "", err
	}
	execPath, execErr := resolveCLIExecutable(cliType)

	// 4. Create conversation_uuid + session_uuid and generate a continuous round number per step.
	// Each re-execution of a step creates a new conversation_uuid, and is stored as
	// it is still the next round of the step, so run_no must not be reset to 1 for a new conversation.
	conversationUUID := uuid.New().String()
	sessionUUID := uuid.New().String()
	if executionMode == "expert_group" {
		if strings.TrimSpace(opts.ExpertChainUUID) == "" {
			opts.ExpertChainUUID = uuid.NewString()
		}
		if opts.ExpertChainRound < 1 {
			opts.ExpertChainRound = 1
		}
	}
	now := nowMillis()
	var runNo int
	if err := o.db.QueryRow(
		`SELECT COALESCE(MAX(run_no), 0) + 1 FROM gt_cli_sessions WHERE step_uuid = ?`,
		stepUUID,
	).Scan(&runNo); err != nil {
		return "", fmt.Errorf("读取步骤轮次失败: %w", err)
	}

	recordType := opts.RecordType
	if recordType == "" {
		recordType = "initial_run"
	}
	if recordType != "initial_run" && recordType != "user_question" {
		return "", fmt.Errorf("无效进度类型")
	}
	progressUUID := uuid.New().String()
	aiProgressUUID := progressUUID
	if recordType == "user_question" {
		aiProgressUUID = uuid.New().String()
	}
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("开启执行事务失败: %w", err)
	}
	defer tx.Rollback()
	if recordType == "user_question" {
		_, err = tx.ExecContext(ctx, `INSERT INTO gt_task_progress
		(uuid, task_uuid, task_pipeline_snapshot_uuid, task_step_uuid, local_step_uuid, cloud_step_id,
		 session_uuid, record_type, user_prompt, cli_type, model_name, status, execution_mode, task_expert_group_snapshot_uuid, created_at, started_at)
		SELECT ?, ?, ?, s.uuid, s.local_step_uuid, s.cloud_step_id, ?, 'user_question', ?, ?, ?, 'created', ?, ?, ?, ?
		FROM gt_task_steps s WHERE s.uuid = ?`, progressUUID, opts.TaskUUID, snapshotUUID, sessionUUID,
			displayPrompt, cliType, modelName, executionMode, expertSnapshotUUID, now, now, stepUUID)
		if err != nil {
			return "", fmt.Errorf("创建用户提问进度失败: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gt_task_progress
		(uuid, task_uuid, task_pipeline_snapshot_uuid, task_step_uuid, local_step_uuid, cloud_step_id,
		 session_uuid, record_type, user_prompt, cli_type, model_name, status, execution_mode, task_expert_group_snapshot_uuid, created_at, started_at)
		SELECT ?, ?, ?, s.uuid, s.local_step_uuid, s.cloud_step_id, ?, ?, ?, ?, ?, 'created', ?, ?, ?, ?
		FROM gt_task_steps s WHERE s.uuid = ?`, aiProgressUUID, opts.TaskUUID, snapshotUUID, sessionUUID,
		"initial_run", "", cliType, modelName, executionMode, expertSnapshotUUID, now, now, stepUUID)
	if err != nil {
		return "", fmt.Errorf("创建任务进度失败: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gt_cli_sessions
		(uuid, task_uuid, step_uuid, task_step_uuid, task_progress_uuid, conversation_uuid, run_no, status,
		 cli_type, model_name, work_dir, prompt_snapshot, execution_mode, expert_chain_uuid, expert_chain_round,
		 trigger_session_uuid, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionUUID, opts.TaskUUID, stepUUID, stepUUID, aiProgressUUID, conversationUUID, runNo,
		SessionStatusCreated, cliType, modelName, opts.WorkDir, promptSnapshot, executionMode, opts.ExpertChainUUID,
		opts.ExpertChainRound, opts.TriggerSessionUUID, now, now, now)
	if err != nil {
		return "", fmt.Errorf("创建 Session 失败: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE gt_task_steps SET execution_status = 'running', started_at = CASE WHEN started_at = 0 THEN ? ELSE started_at END, updated_at = ? WHERE uuid = ?`, now, now, stepUUID); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE gt_tasks SET execution_status = 'running', started_at = CASE WHEN started_at = 0 THEN ? ELSE started_at END, updated_at = ? WHERE uuid = ?`, now, now, opts.TaskUUID); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("提交执行事务失败: %w", err)
	}

	// Broadcast the state change
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(opts.TaskUUID, opts.StepKey, sessionUUID, SessionStatusRunning)
	}
	if execErr != nil {
		go o.failSessionWithError(sessionUUID, opts.TaskUUID, opts.StepKey, "", execErr.Error())
		return sessionUUID, nil
	}

	// 7. Start the CLI process asynchronously
	go o.runCLIProcess(context.WithoutCancel(ctx), sessionUUID, opts, cliType, execPath, promptSnapshot, false, "", modelName)

	return sessionUUID, nil
}

// ContinueConversation creates a new locally recorded round. It resumes the
// same native conversation when the CLI is unchanged and a native session ID
// exists; otherwise it starts a fresh native conversation while preserving the
// product-level conversation UUID and round sequence.
func (o *Orchestrator) ContinueConversation(ctx context.Context, opts ContinueConversationOptions) (string, error) {
	var taskUUID string
	if err := o.db.QueryRowContext(ctx, `SELECT task_uuid FROM gt_cli_sessions WHERE uuid=?`, opts.ParentSessionUUID).Scan(&taskUUID); err == sql.ErrNoRows {
		return "", fmt.Errorf("历史会话不存在")
	} else if err != nil {
		return "", fmt.Errorf("读取历史会话失败: %w", err)
	}
	release := o.executionGates.acquire(taskUUID, false)
	defer release()
	return o.continueConversation(ctx, opts)
}

func (o *Orchestrator) continueConversation(ctx context.Context, opts ContinueConversationOptions) (string, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("问题内容不能为空")
	}
	displayPrompt := strings.TrimSpace(opts.DisplayPrompt)
	if displayPrompt == "" {
		displayPrompt = prompt
	}
	var taskUUID, stepUUID, stepKey, workDir, parentStatus, conversationUUID, externalSessionID, parentCLIType, parentModelName, snapshotUUID, expertSnapshotUUID, executionMode string
	err := o.db.QueryRowContext(ctx,
		`SELECT s.task_uuid, s.step_uuid, ts.step_key, s.work_dir, s.status, s.conversation_uuid,
		        s.external_session_id, s.cli_type, s.model_name, ts.task_pipeline_snapshot_uuid,
		        ts.task_expert_group_snapshot_uuid, COALESCE(NULLIF(ts.execution_mode,''), t.execution_mode)
		 FROM gt_cli_sessions s
		 JOIN gt_task_steps ts ON ts.uuid = s.step_uuid
		 JOIN gt_tasks t ON t.uuid=s.task_uuid
		 WHERE s.uuid = ?`,
		opts.ParentSessionUUID,
	).Scan(&taskUUID, &stepUUID, &stepKey, &workDir, &parentStatus, &conversationUUID,
		&externalSessionID, &parentCLIType, &parentModelName, &snapshotUUID, &expertSnapshotUUID, &executionMode)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("历史会话不存在")
	}
	if err != nil {
		return "", fmt.Errorf("读取历史会话失败: %w", err)
	}
	if !IsTerminalStatus(parentStatus) {
		return "", fmt.Errorf("当前一轮仍在执行，请等待完成或先终止")
	}
	cliType, modelName, nativeSessionID, resumeNative, err := resolveContinuationRuntime(
		parentCLIType,
		parentModelName,
		externalSessionID,
		opts.CLIType,
		opts.Model,
	)
	if err != nil {
		return "", err
	}
	runOpts := RunStepOptions{TaskUUID: taskUUID, StepUUID: stepUUID, StepKey: stepKey,
		WorkDir: workDir, RequestID: opts.RequestID, CLIType: cliType, Model: modelName,
		UserPrompt: displayPrompt, RecordType: "user_question", ExecutionMode: executionMode,
		ExpertChainUUID: opts.ExpertChainUUID, ExpertChainRound: opts.ExpertChainRound, TriggerSessionUUID: opts.TriggerSessionUUID}
	if err = o.validateRun(ctx, runOpts); err != nil {
		return "", err
	}
	// Resume sends only the user's raw message because the native conversation
	// already carries context. Switching CLI (or lacking a native session ID)
	// starts a fresh native conversation and rebuilds the complete step prompt.
	// Both paths keep the parent's product-level conversation_uuid and advance
	// run_no, as required by the local conversation and CloudSync contract.
	storedPromptSnapshot := displayPrompt
	executionPromptSnapshot := prompt
	if !resumeNative {
		storedPromptSnapshot, err = BuildRoundPrompt(ctx, o.db, executionMode, taskUUID, stepUUID, displayPrompt)
		if err != nil {
			return "", err
		}
		executionPromptSnapshot, err = BuildRoundPrompt(ctx, o.db, executionMode, taskUUID, stepUUID, prompt)
		if err != nil {
			return "", err
		}
	}
	execPath, execErr := resolveCLIExecutable(cliType)
	now := nowMillis()
	sessionUUID := uuid.NewString()
	if executionMode == "expert_group" {
		if opts.ExpertChainUUID == "" {
			opts.ExpertChainUUID = uuid.NewString()
		}
		if opts.ExpertChainRound < 1 {
			opts.ExpertChainRound = 1
		}
	}
	progressUUID := uuid.NewString()   // 用户提问 progress
	aiProgressUUID := uuid.NewString() // AI 执行 progress

	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("开启继续会话事务失败: %w", err)
	}
	defer tx.Rollback()
	var runNo int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_no), 0) + 1 FROM gt_cli_sessions WHERE step_uuid = ?`, stepUUID).Scan(&runNo); err != nil {
		return "", fmt.Errorf("读取步骤轮次失败: %w", err)
	}
	var stepStatus, taskStatus string
	var activeCount int
	if err = tx.QueryRowContext(ctx, `SELECT ts.status, t.status,
		(SELECT COUNT(*) FROM gt_cli_sessions a WHERE a.task_uuid=t.uuid AND a.status IN ('created','running','waiting_input','stop_requested'))
		FROM gt_tasks t JOIN gt_task_steps ts ON ts.task_uuid=t.uuid
		WHERE t.uuid=? AND ts.uuid=?`, taskUUID, stepUUID).Scan(&stepStatus, &taskStatus, &activeCount); err != nil {
		return "", fmt.Errorf("重新校验任务步骤失败: %w", err)
	}
	// 已完成的编排允许继续对话；仅拦截尚未开始的后续步骤
	if stepStatus != "active" && stepStatus != "completed" {
		return "", fmt.Errorf("只能继续已开始或已完成的 Agent 编排")
	}
	if activeCount > 0 {
		return "", fmt.Errorf("任务已有活动 Session")
	}
	// 1. 用户提问 progress（record_type = user_question, user_prompt = 用户输入，显示为用户消息气泡）
	if !opts.InternalHandoff {
		_, err = tx.ExecContext(ctx, `INSERT INTO gt_task_progress
		(uuid, task_uuid, task_pipeline_snapshot_uuid, task_step_uuid, local_step_uuid, cloud_step_id,
		 session_uuid, record_type, user_prompt, cli_type, model_name, status, execution_mode, external_session_id, task_expert_group_snapshot_uuid, created_at, started_at)
		SELECT ?, ?, ?, s.uuid, s.local_step_uuid, s.cloud_step_id, ?, 'user_question', ?, ?, ?, 'created', ?, ?, ?, ?, ?
		FROM gt_task_steps s WHERE s.uuid = ?`, progressUUID, taskUUID, snapshotUUID, sessionUUID,
			displayPrompt, cliType, modelName, executionMode, nativeSessionID, expertSnapshotUUID, now, now, stepUUID)
		if err != nil {
			return "", fmt.Errorf("创建用户提问进度失败: %w", err)
		}
	}
	// 2. AI 执行 progress（record_type = initial_run, user_prompt = '', 显示为 AI 执行气泡）
	_, err = tx.ExecContext(ctx, `INSERT INTO gt_task_progress
		(uuid, task_uuid, task_pipeline_snapshot_uuid, task_step_uuid, local_step_uuid, cloud_step_id,
		 session_uuid, record_type, user_prompt, cli_type, model_name, status, execution_mode, external_session_id, task_expert_group_snapshot_uuid, created_at, started_at)
		SELECT ?, ?, ?, s.uuid, s.local_step_uuid, s.cloud_step_id, ?, 'initial_run', '', ?, ?, 'created', ?, ?, ?, ?, ?
		FROM gt_task_steps s WHERE s.uuid = ?`, aiProgressUUID, taskUUID, snapshotUUID, sessionUUID,
		cliType, modelName, executionMode, nativeSessionID, expertSnapshotUUID, now, now, stepUUID)
	if err != nil {
		return "", fmt.Errorf("创建 AI 执行进度失败: %w", err)
	}
	// 3. CLI 会话记录，task_progress_uuid 指向 AI 执行 progress
	_, err = tx.ExecContext(ctx, `INSERT INTO gt_cli_sessions
		(uuid, task_uuid, step_uuid, task_step_uuid, task_progress_uuid, conversation_uuid, parent_session_uuid,
		 run_no, status, cli_type, model_name, external_session_id, work_dir, prompt_snapshot,
		 execution_mode, expert_chain_uuid, expert_chain_round, trigger_session_uuid, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionUUID, taskUUID, stepUUID, stepUUID, aiProgressUUID, conversationUUID, opts.ParentSessionUUID,
		runNo, SessionStatusCreated, cliType, modelName, nativeSessionID, workDir, storedPromptSnapshot,
		executionMode, opts.ExpertChainUUID, opts.ExpertChainRound, opts.TriggerSessionUUID, now, now, now)
	if err != nil {
		return "", fmt.Errorf("创建继续会话 Session 失败: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE gt_task_steps SET execution_status='running', updated_at=? WHERE uuid=?`, now, stepUUID); err != nil {
		return "", err
	}
	// done 任务上的补充对话不回退任务整体状态，仅刷新执行状态；其余状态恢复为 active
	if _, err = tx.ExecContext(ctx, `UPDATE gt_tasks SET status = CASE WHEN status = 'done' THEN status ELSE 'active' END, execution_status='running', updated_at=? WHERE uuid=?`, now, taskUUID); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("提交继续会话事务失败: %w", err)
	}

	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, stepKey, sessionUUID, SessionStatusRunning)
	}
	if execErr != nil {
		go o.failSessionWithError(sessionUUID, taskUUID, stepKey, nativeSessionID, execErr.Error())
		return sessionUUID, nil
	}
	go o.runCLIProcess(context.WithoutCancel(ctx), sessionUUID, runOpts, cliType, execPath, executionPromptSnapshot, resumeNative, nativeSessionID, modelName)
	return sessionUUID, nil
}

func resolveContinuationRuntime(parentCLIType, parentModelName, externalSessionID, requestedCLIType, requestedModel string) (cliType, modelName, nativeSessionID string, resumeNative bool, err error) {
	parentCLIType = strings.TrimSpace(parentCLIType)
	parentModelName = strings.TrimSpace(parentModelName)
	requestedCLIType = strings.TrimSpace(requestedCLIType)
	requestedModel = strings.TrimSpace(requestedModel)

	cliType = requestedCLIType
	if cliType == "" {
		cliType = parentCLIType
	}
	modelName = requestedModel
	if modelName == "" {
		modelName = parentModelName
	}
	if requestedCLIType != "" && requestedCLIType != parentCLIType && requestedModel == "" {
		return "", "", "", false, fmt.Errorf("切换 CLI 时必须同时选择模型")
	}

	resumeNative = cliType == parentCLIType && strings.TrimSpace(externalSessionID) != ""
	if resumeNative {
		nativeSessionID = strings.TrimSpace(externalSessionID)
	}
	return cliType, modelName, nativeSessionID, resumeNative, nil
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

	// Create the adapter：按 CLI 类型查注册表，新增 CLI 无需改动编排层
	adapter, ok := newAdapter(cliType)
	if !ok {
		o.failSessionWithError(sessionUUID, opts.TaskUUID, opts.StepKey, "", "不支持的 CLI 类型: "+cliType)
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
	applog.Info("[Orchestrator] CLI 开始执行",
		"session", sessionUUID,
		"cliType", cliType,
		"model", model,
		"workDir", opts.WorkDir,
		"resume", resume,
		"taskUUID", opts.TaskUUID,
		"stepKey", opts.StepKey,
		"externalSessionID", externalSessionID,
		"prompt", prompt,
	)
	runOptions := executor.RunOptions{
		ExecPath:     execPath,
		Prompt:       prompt,
		WorkDir:      opts.WorkDir,
		ModelProfile: model,
	}
	eventCh, err := startAdapterConversation(innerCtx, adapter, runOptions, resume, externalSessionID)
	if err != nil {
		applog.Warn("[Orchestrator] 启动 CLI 失败", "error", err)
		o.failSessionWithError(sessionUUID, opts.TaskUUID, opts.StepKey, "", err.Error())
		return
	}
	// 启动广播（startSession/ContinueConversation）发生在状态翻转之前；翻转后需补发广播，
	// 否则前端在 WS 推送模式下不轮询，会一直停留在 created（等待执行）。
	// 仅当本次 UPDATE 真正把 created 翻转为 running 时广播，避免与 StopSession 竞争时误报。
	flipped, err := o.db.Exec(
		`UPDATE gt_cli_sessions SET status = ?, updated_at = ? WHERE uuid = ? AND status = ?`,
		SessionStatusRunning, nowMillis(), sessionUUID, SessionStatusCreated,
	)
	o.db.Exec(`UPDATE gt_task_progress SET status = 'running' WHERE uuid = (SELECT task_progress_uuid FROM gt_cli_sessions WHERE uuid = ?) AND status = 'created'`, sessionUUID)
	if err == nil {
		if affected, err := flipped.RowsAffected(); err == nil && affected > 0 && o.stateBroadcaster != nil {
			o.stateBroadcaster(opts.TaskUUID, opts.StepKey, sessionUUID, SessionStatusRunning)
		}
	}

	applog.Info("[Orchestrator] CLI 会话已启动，等待输出",
		"session", sessionUUID,
		"cliType", cliType,
		"model", model,
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
	eventSequence := 0

	// Latest CLI activity (tool call / output) for the progress panel.
	// Persisted with throttling to avoid a DB write per event; the final event
	// is flushed after the loop so the panel always shows the last activity.
	latestEventType := ""
	latestEventContent := ""
	latestEventAt := int64(0)
	lastPersistedEventAt := int64(0)
	persistLatestEvent := func() {
		if latestEventAt <= lastPersistedEventAt || latestEventContent == "" {
			return
		}
		content := truncate(latestEventContent, 300)
		o.db.Exec(
			`UPDATE gt_cli_sessions SET latest_event_type = ?, latest_event_content = ?, latest_event_at = ?, updated_at = ? WHERE uuid = ?`,
			latestEventType, content, latestEventAt, nowMillis(), sessionUUID,
		)
		lastPersistedEventAt = latestEventAt
		// Push alongside the throttled DB write so task views can update the
		// running-step subtitle without polling /progress.
		if o.activityBroadcaster != nil {
			o.activityBroadcaster(opts.TaskUUID, sessionUUID, latestEventType, content, latestEventAt)
		}
	}
	trackLatestEvent := func(eventType, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		latestEventType = eventType
		latestEventContent = content
		latestEventAt = nowMillis()
		if latestEventAt-lastPersistedEventAt >= latestEventPersistIntervalMs {
			persistLatestEvent()
		}
	}

	// 执行过程回放表：思考/工具调用事件按序写入，供动态区展开回看与 WS 补发
	eventStore := log.NewEventStore(o.db)

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
				o.recomputeSession(opts.TaskUUID, opts.StepKey)
			}
			continue
		}

		// 过程事件（思考 / 中间输出 / 工具调用 / 工具结果 / 权限请求）留存到
		// gt_session_events（sequence 与广播一致），供动态区“执行过程”历史回放与
		// WS 断线补发。最终输出仍以 final_result 为准：前端在回放时会排除该轮
		// 最后一条 message，避免同一条回答在过程与结论里出现两次。
		eventSequence++
		if o.eventBroadcaster != nil {
			o.eventBroadcaster(sessionUUID, eventSequence, event)
		}
		if limit, persist := sessionEventContentLimit(event.Type); persist {
			stored := event
			stored.Content = truncate(event.Content, limit)
			if payload, err := json.Marshal(stored); err == nil {
				if err := eventStore.Append(log.EventLogEntry{
					SessionUUID: sessionUUID,
					Sequence:    eventSequence,
					EventType:   event.Type,
					Payload:     string(payload),
				}); err != nil {
					applog.Warn("[Orchestrator] 执行过程事件写入失败", "session", sessionUUID, "error", err)
				}
			}
		}

		// Log CLI output
		content := strings.TrimSpace(event.Content)
		switch event.Type {
		case executor.EventMessage:
			if content != "" {
				applog.Debug("[Orchestrator] CLI 消息输出", "session", sessionUUID, "content", truncate(content, 2048))
				trackLatestEvent(executor.EventMessage, content)
			}
		case executor.EventThinking:
			applog.Debug("[Orchestrator] CLI 思考过程", "session", sessionUUID, "content", truncate(content, 1024))
			trackLatestEvent(executor.EventThinking, content)
		case executor.EventToolCall:
			applog.Debug("[Orchestrator] CLI 工具调用", "session", sessionUUID, "content", truncate(content, 1024))
			trackLatestEvent(executor.EventToolCall, content)
		case executor.EventToolResult:
			applog.Debug("[Orchestrator] CLI 工具结果", "session", sessionUUID, "content", truncate(content, 1024))
			trackLatestEvent(executor.EventToolResult, content)
		case executor.EventPermission:
			// 非交互模式下 CLI 会停下来等待授权；不上报活动行时，界面表现为「一直在跑」。
			applog.Info("[Orchestrator] CLI 请求工具权限", "session", sessionUUID, "content", truncate(content, 512))
			trackLatestEvent(executor.EventPermission, content)
		case executor.EventUsage:
			applog.Debug("[Orchestrator] CLI Token 用量", "session", sessionUUID, "inputTokens", event.InputTokens, "outputTokens", event.OutputTokens)
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
		// Keep only the latest AI-visible message in memory; finishSession writes it
		// once as the final local result. Intermediate events are never persisted or uploaded.
		if event.Type == executor.EventMessage && strings.TrimSpace(event.Content) != "" {
			responseContent = strings.TrimSpace(event.Content)
		}

		// Completion event
		if event.Type == executor.EventComplete {
			if event.SessionID != "" {
				sessionID = event.SessionID
			}
			if strings.TrimSpace(event.Content) != "" {
				responseContent = strings.TrimSpace(event.Content)
			}
			// complete carries the total of this execution; overwrite rather than accumulate
			inputTokens = event.InputTokens
			outputTokens = event.OutputTokens
			completeTokensSeen = true
			break
		}
		if event.Type == executor.EventError {
			applog.Error("[Orchestrator] CLI 返回错误事件",
				"session", sessionUUID,
				"error", event.Error,
				"responseContent", truncate(responseContent, 2048),
			)
			status := SessionStatusFailed
			var stopRequestedAt int64
			_ = o.db.QueryRow(`SELECT stop_requested_at FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&stopRequestedAt)
			if stopRequestedAt > 0 {
				status = SessionStatusStopped
			}
			// Abnormal termination cannot get complete; fall back to mid-way usage accumulation
			persistLatestEvent()
			o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, status, sessionID, usageInputTokens, usageOutputTokens, responseContent)
			return
		}
	}

	// Flush the last activity so the progress panel keeps the latest event
	persistLatestEvent()

	var stopRequestedAt int64
	_ = o.db.QueryRow(`SELECT stop_requested_at FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&stopRequestedAt)
	if !completeTokensSeen {
		status := SessionStatusFailed
		if stopRequestedAt > 0 {
			status = SessionStatusStopped
		} else {
			const incompleteReplyMessage = "CLI 未返回完整完成事件"
			applog.Error("[Orchestrator] CLI 回复不完整",
				"session", sessionUUID,
				"taskUUID", opts.TaskUUID,
				"stepKey", opts.StepKey,
			)
			o.db.Exec(
				`UPDATE gt_cli_sessions SET error_message = ?, updated_at = ? WHERE uuid = ?`,
				incompleteReplyMessage, nowMillis(), sessionUUID,
			)
		}
		o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, status, sessionID, usageInputTokens, usageOutputTokens, responseContent)
		return
	}

	status := SessionStatusSuccess
	if stopRequestedAt > 0 {
		status = SessionStatusStopped
	}
	o.finishSession(sessionUUID, opts.TaskUUID, opts.StepKey, status, sessionID, inputTokens, outputTokens, responseContent)

	applog.Info("[Orchestrator] CLI 执行结束",
		"session", sessionUUID,
		"status", status,
		"responseContent", truncate(responseContent, 2048),
	)
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

// recomputeSession updates the derived step and task execution states after a session change.
func (o *Orchestrator) recomputeSession(taskUUID, stepKey string) {
	tx, err := o.db.Begin()
	if err != nil {
		applog.Warn("[Orchestrator] 开启 Session 聚合事务失败", "error", err)
		return
	}
	defer tx.Rollback()
	if err := RecomputeStepStatus(tx, taskUUID, stepKey); err != nil {
		applog.Warn("[Orchestrator] 聚合 Session 步骤状态失败", "error", err)
		return
	}
	if err := RecomputeTaskStatus(tx, taskUUID); err != nil {
		applog.Warn("[Orchestrator] 聚合 Session 任务状态失败", "error", err)
		return
	}
	if err := tx.Commit(); err != nil {
		applog.Warn("[Orchestrator] 提交 Session 聚合状态失败", "error", err)
	}
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

// failSessionWithError records the failure reason and finishes the Session as failed.
//
// It is used by the synchronous failure paths in runCLIProcess (adapter creation /
// CLI start failure or unsupported CLI type), where
// the adapter cannot produce an error event on the event channel. Without this, the
// task execution window only sees the "failed" state change with no failure details.
func (o *Orchestrator) failSessionWithError(sessionUUID, taskUUID, stepKey, externalSessionID, message string) {
	message = strings.TrimSpace(message)
	if message != "" {
		applog.Error("[Orchestrator] CLI 执行失败",
			"session", sessionUUID,
			"taskUUID", taskUUID,
			"stepKey", stepKey,
			"error", message,
		)
		o.db.Exec(
			`UPDATE gt_cli_sessions SET error_message = ?, updated_at = ? WHERE uuid = ?`,
			truncate(message, 8192), nowMillis(), sessionUUID,
		)
		event := executor.ExecutorEvent{
			Type:      executor.EventError,
			Error:     message,
			Timestamp: nowMillis(),
		}
		if o.eventBroadcaster != nil {
			o.eventBroadcaster(sessionUUID, 1, event)
		}
	}
	o.finishSession(sessionUUID, taskUUID, stepKey, SessionStatusFailed, externalSessionID, 0, 0, "")
}

// finishSession completes the Session and updates its status
func (o *Orchestrator) finishSession(sessionUUID, taskUUID, stepKey, status, externalSessionID string, inputTokens, outputTokens int, responseContent string) {
	now := nowMillis()
	var errorMessage string
	_ = o.db.QueryRow(`SELECT error_message FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&errorMessage)
	finalResult := strings.TrimSpace(responseContent)
	if status != SessionStatusSuccess && strings.TrimSpace(errorMessage) != "" {
		finalResult = strings.TrimSpace(errorMessage)
	}
	if finalResult == "" && status != SessionStatusSuccess {
		finalResult = "CLI 执行" + status
	}
	tx, err := o.db.Begin()
	if err != nil {
		applog.Warn("[Orchestrator] 开启完成事务失败", "error", err)
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE gt_cli_sessions
		 SET status = ?, result_status = ?, final_result = ?,
		     external_session_id = CASE WHEN ? <> '' THEN ? ELSE external_session_id END,
		     input_tokens = ?, output_tokens = ?, total_tokens = ?,
		     finished_at = ?, duration_ms = CASE WHEN started_at > 0 THEN ? - started_at ELSE 0 END, updated_at = ?
		 WHERE uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		status, status, truncate(finalResult, 64*1024), externalSessionID, externalSessionID,
		inputTokens, outputTokens, inputTokens+outputTokens, now, now, now, sessionUUID)
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

	if _, err = tx.Exec(`UPDATE gt_task_progress SET status = ?, finished_at = ?,
		final_result = CASE WHEN record_type = 'user_question' THEN final_result ELSE ? END,
		external_session_id = CASE WHEN ? <> '' THEN ? ELSE external_session_id END
		WHERE session_uuid = ?`, status, now, truncate(finalResult, 64*1024), externalSessionID, externalSessionID, sessionUUID); err != nil {
		applog.Warn("[Orchestrator] 更新任务进度失败", "error", err)
		return
	}
	if _, err = tx.Exec(`UPDATE gt_task_progress
		SET dispatch_status='routing_pending', dispatch_message=?
		WHERE session_uuid=? AND record_type<>'user_question'
		  AND EXISTS (SELECT 1 FROM gt_cli_sessions s WHERE s.uuid=? AND s.execution_mode='expert_group')`,
		expertDispatchCodeRoutingPending, sessionUUID, sessionUUID); err != nil {
		applog.Warn("[Orchestrator] 标记专家团自动路由失败", "error", err)
		return
	}
	if err = RecomputeStepStatus(tx, taskUUID, stepKey); err != nil {
		applog.Warn("[Orchestrator] 重算 Step 状态失败", "error", err)
		return
	}
	if _, err = tx.Exec(`UPDATE gt_task_steps SET output_summary = CASE WHEN ? = 'success' THEN ? ELSE output_summary END,
		error_summary = CASE WHEN ? = 'success' THEN '' ELSE ? END, updated_at = ?
		WHERE task_uuid = ? AND step_key = ?`, status, truncate(finalResult, 64*1024), status, truncate(finalResult, 8192), now, taskUUID, stepKey); err != nil {
		return
	}
	if err = RecomputeTaskStatus(tx, taskUUID); err != nil {
		applog.Warn("[Orchestrator] 重算 Task 状态失败", "error", err)
		return
	}
	// done 任务上的补充对话失败时，不得把任务整体回退为 blocked
	if _, err = tx.Exec(`UPDATE gt_tasks SET status = CASE WHEN ? = 'success' OR status = 'done' THEN status ELSE 'blocked' END, updated_at = ? WHERE uuid = ?`, status, now, taskUUID); err != nil {
		applog.Warn("[Orchestrator] 更新任务业务状态失败", "error", err)
		return
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO gt_task_notifications
		(uuid, task_uuid, task_step_uuid, step_order, progress_uuid, session_uuid, terminal_status, title, summary, created_at)
		SELECT ?, s.task_uuid, s.uuid, s.step_order, p.uuid, ?, ?,
		       t.title || ' · ' || s.name, ?, ?
		FROM gt_cli_sessions cs
		JOIN gt_task_progress p ON p.uuid = cs.task_progress_uuid
		JOIN gt_task_steps s ON s.uuid = cs.step_uuid
		JOIN gt_tasks t ON t.uuid = cs.task_uuid
		WHERE cs.uuid = ?`, uuid.New().String(), sessionUUID, status, truncate(finalResult, 512), now, sessionUUID)
	if err != nil {
		applog.Warn("[Orchestrator] 创建任务通知失败", "error", err)
		return
	}
	if err = tx.Commit(); err != nil {
		applog.Warn("[Orchestrator] 提交完成事务失败", "error", err)
		return
	}

	// Persist the conversation summary consumed by the cloud task sync (idempotent per
	// session). Failed sessions fall back to the final result text so the cloud-side
	// assistant message is never empty.
	summaryContent := strings.TrimSpace(responseContent)
	if summaryContent == "" {
		summaryContent = finalResult
	}
	if err := o.ensureSessionSummary(sessionUUID, stepKey, summaryContent, inputTokens, outputTokens, now); err != nil {
		applog.Warn("[Orchestrator] 写入会话摘要失败", "session", sessionUUID, "error", err)
	}

	// Broadcast the state change
	if o.stateBroadcaster != nil {
		o.stateBroadcaster(taskUUID, stepKey, sessionUUID, status)
	}
	for _, listener := range o.stateListeners {
		listener(taskUUID, stepKey, sessionUUID, status)
	}
	go o.routeExpertCompletion(sessionUUID, taskUUID, status, finalResult)
}

func truncate(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
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
		applog.Warn("[Orchestrator] 记录停止请求失败", "session", sessionUUID, "ctx_err", ctx.Err(), "error", err)
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
	o.db.Exec(`UPDATE gt_cli_sessions SET error_message = CASE WHEN error_message <> '' THEN error_message ELSE ? END WHERE uuid = ?`, "用户手动终止", sessionUUID)
	o.finishSession(sessionUUID, taskUUID, stepKey, SessionStatusStopped, "", 0, 0, "")
	return nil
}

// validateRun pre-flight validation
func (o *Orchestrator) validateRun(ctx context.Context, opts RunStepOptions) error {
	// Check task exists
	var stepStatus, executionMode string
	err := o.db.QueryRowContext(ctx, `SELECT s.status, COALESCE(NULLIF(s.execution_mode,''), t.execution_mode)
		FROM gt_task_steps s
		JOIN gt_tasks t ON t.uuid=s.task_uuid
		WHERE s.task_uuid = ? AND s.uuid = ?`, opts.TaskUUID, opts.StepUUID).Scan(&stepStatus, &executionMode)
	if err == sql.ErrNoRows {
		return fmt.Errorf("任务不存在: %s", opts.TaskUUID)
	}
	if err != nil {
		return fmt.Errorf("查询任务失败: %w", err)
	}
	// 已完成的编排允许再次执行/继续对话，不改变流水线进度；仅拦截尚未开始的后续步骤
	if executionMode != "expert_group" && stepStatus != "active" && stepStatus != "completed" {
		return fmt.Errorf("只能执行任务进行中或已完成的 Agent 编排")
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

// StopTaskSessions stops all active CLI sessions for a given task.
func (o *Orchestrator) StopTaskSessions(ctx context.Context, taskUUID string) error {
	release := o.executionGates.acquire(taskUUID, false)
	defer release()
	return o.stopTaskSessions(ctx, taskUUID)
}

func (o *Orchestrator) stopTaskSessions(ctx context.Context, taskUUID string) error {
	rows, err := o.db.QueryContext(ctx,
		`SELECT uuid FROM gt_cli_sessions WHERE task_uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		taskUUID)
	if err != nil {
		return fmt.Errorf("查询任务活动会话失败: %w", err)
	}
	// 先把 UUID 全部取出并关闭游标，再逐个停止。SQLite 是单连接池
	// （SetMaxOpenConns(1)），游标未关闭前持有唯一连接，StopSession 内部
	// 的查询/更新会排队等连接，形成死锁：接口挂起直到客户端断开才报
	// context canceled。
	var sessionUUIDs []string
	for rows.Next() {
		var sessionUUID string
		if err := rows.Scan(&sessionUUID); err != nil {
			rows.Close()
			return fmt.Errorf("读取任务活动会话失败: %w", err)
		}
		sessionUUIDs = append(sessionUUIDs, sessionUUID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历任务活动会话失败: %w", err)
	}
	applog.Info("[Orchestrator] 停止任务会话", "task_uuid", taskUUID, "active_sessions", len(sessionUUIDs))

	var firstErr error
	for _, sessionUUID := range sessionUUIDs {
		start := nowMillis()
		if err := o.StopSession(ctx, sessionUUID); err != nil {
			applog.Warn("[Orchestrator] 停止会话失败", "session", sessionUUID, "error", err, "cost_ms", nowMillis()-start)
			if firstErr == nil {
				firstErr = fmt.Errorf("终止会话 %s 失败: %w", sessionUUID, err)
			}
			continue
		}
		applog.Info("[Orchestrator] 停止会话成功", "session", sessionUUID, "cost_ms", nowMillis()-start)
	}
	return firstErr
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

// resolveCLIExecutable locates the executable for the CLI type.
// It delegates to executor.ResolveCLIExecutable, which probes PATH as well as
// well-known install directories (e.g. %LOCALAPPDATA%\cursor-agent\agent.cmd),
// so CLIs installed without being added to PATH still work.
func resolveCLIExecutable(cliType string) (string, error) {
	return executor.ResolveCLIExecutable(cliType)
}
