package cloudsync

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
)

// cursorTTL is the in-memory incremental cursor lifetime: after this idle window a
// task's next push falls back to a full payload (the cloud upsert is idempotent).
const cursorTTL = 24 * time.Hour

// Pusher aggregates local task execution results and pushes them incrementally to
// the cloud via REST. It replaces the removed WebSocket sync channel: each push
// carries the full step list and task-level counters; conversation content travels
// through the separate step-level session sync service.
type Pusher struct {
	mu     sync.Mutex
	cursor map[string]pushCursor // taskUUID → last pushed conversation watermark
}

type pushCursor struct {
	lastCreatedAt int64 // ms watermark of the last pushed conversation summary
	lastRevision  int64 // last cloud-accepted local_revision (monotonic guard)
	at            time.Time
}

// NewPusher creates a task-result pusher.
func NewPusher() *Pusher {
	return &Pusher{cursor: make(map[string]pushCursor)}
}

// defaultPusher is shared by the HTTP-triggered pushes and the session state-change
// listener so both advance the same per-task watermark and revision guard.
var defaultPusher = NewPusher()

// DefaultPusher returns the process-wide task-result pusher.
func DefaultPusher() *Pusher {
	return defaultPusher
}

// PushTaskSync aggregates the task's latest state and pushes the increment to the cloud.
// It is safe for concurrent calls; the watermark is only advanced after a successful push.
func (p *Pusher) PushTaskSync(ctx context.Context, db *sql.DB, client *cloud.Client, taskUUID string) error {
	if db == nil || client == nil {
		return fmt.Errorf("任务数据库或云端客户端未初始化")
	}
	watermark := p.watermark(taskUUID)
	payload, err := BuildTaskSync(ctx, db, taskUUID, watermark)
	if err != nil {
		return err
	}
	if payload == nil {
		applog.Warn("[CloudSync] 任务投影未生成（BuildTaskSync 返回空），本次未向云端推送，详见上方跳过原因", "task_uuid", taskUUID)
		return nil
	}
	// Monotonic revision guard: two pushes within the same millisecond would otherwise
	// share a revision and the cloud rejects the second one as a hash conflict.
	payload.LocalRevision = p.reserveRevision(taskUUID, payload.LocalRevision)
	result, err := client.PushPipelineTaskSync(ctx, *payload)
	if err != nil {
		return err
	}
	p.commitRevision(taskUUID, payload.LocalRevision)
	if result.Result == "duplicate" {
		// The cloud already has this revision; keep the local watermark unchanged so the
		// next push retries with a newer revision instead of silently dropping data.
		return nil
	}
	p.advanceWatermark(taskUUID, payload.LastLocalUpdatedAt)
	return nil
}

// DeleteTaskSync deletes the cloud projection of a locally deleted task.
func (p *Pusher) DeleteTaskSync(ctx context.Context, client *cloud.Client, taskUUID string) error {
	if client == nil {
		return fmt.Errorf("云端客户端未初始化")
	}
	p.mu.Lock()
	delete(p.cursor, taskUUID)
	p.mu.Unlock()
	return client.DeletePipelineTask(ctx, taskUUID)
}

func (p *Pusher) watermark(taskUUID string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.cursor[taskUUID]
	if !ok || time.Since(c.at) > cursorTTL {
		return 0
	}
	return c.lastCreatedAt
}

func (p *Pusher) advanceWatermark(taskUUID string, lastLocalUpdatedAt string) {
	watermark := parseMillis(lastLocalUpdatedAt)
	if watermark <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.cursor[taskUUID]
	if watermark > c.lastCreatedAt {
		c.lastCreatedAt = watermark
		c.at = time.Now()
		p.cursor[taskUUID] = c
	}
}

// reserveRevision returns a task revision strictly greater than any revision already
// accepted by the cloud for this task, protecting against same-millisecond pushes.
func (p *Pusher) reserveRevision(taskUUID string, candidate int64) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if last := p.cursor[taskUUID].lastRevision; candidate <= last {
		return last + 1
	}
	return candidate
}

// commitRevision records the revision accepted by the cloud after a successful push.
func (p *Pusher) commitRevision(taskUUID string, revision int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.cursor[taskUUID]
	if revision > c.lastRevision {
		c.lastRevision = revision
		p.cursor[taskUUID] = c
	}
}

// BuildTaskSync aggregates the task's incremental sync payload from the local database.
// Returns nil when the task is not a cloud task (or is already gone).
func BuildTaskSync(ctx context.Context, db *sql.DB, taskUUID string, sinceCreatedAt int64) (*cloud.PipelineTaskSync, error) {
	base, err := loadTaskBase(ctx, db, taskUUID)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return nil, nil
	}
	if base.pipeline.ID <= 0 || base.workItem.ID <= 0 || base.workItem.WorkspaceID <= 0 {
		// Pipeline not assigned yet (or legacy row missing work-item links): the cloud
		// would reject the payload, so skip the push until the task is complete.
		applog.Warn("[CloudSync] 任务投影跳过推送：流水线或工作项未就绪（未分配流水线快照/工作项关联缺失），云端不会创建任务记录",
			"task_uuid", taskUUID, "pipeline_id", base.pipeline.ID, "work_item_id", base.workItem.ID, "workspace_id", base.workItem.WorkspaceID)
		return nil, nil
	}
	steps, err := loadTaskSteps(ctx, db, taskUUID)
	if err != nil {
		return nil, err
	}
	payload := cloud.PipelineTaskSync{
		LocalTaskUUID:      taskUUID,
		LocalRevision:      time.Now().UnixMilli(),
		ProjectionHash:     projectionHash(base, steps),
		WorkItem:           base.workItem,
		Pipeline:           base.pipeline,
		TaskStatus:         base.taskStatus,
		ExecutionStatus:    base.executionStatus,
		CurrentStepKey:     base.currentStepKey,
		ExecutionSummary:   base.executionSummary,
		StartedAt:          base.startedAt,
		FinishedAt:         base.finishedAt,
		LastLocalUpdatedAt: base.lastLocalUpdatedAt,
		Usage:              base.usage,
		Steps:              steps,
		DeletedStepKeys:    []string{},
	}
	return &payload, nil
}

// ======================== Task base aggregation ========================

type taskBase struct {
	workItem           cloud.PipelineTaskWorkItem
	pipeline           cloud.PipelineTaskPipeline
	taskStatus         string
	executionStatus    string
	currentStepKey     string
	executionSummary   string
	startedAt          *string
	finishedAt         *string
	lastLocalUpdatedAt string
	usage              cloud.PipelineTaskUsage
}

func loadTaskBase(ctx context.Context, db *sql.DB, taskUUID string) (*taskBase, error) {
	var sourceType, workItemType, workItemID, workspaceID, title string
	var taskStatus, execStatus, currentStepUUID, cliType string
	var pipelineID, pipelineName string
	var inputTokens, outputTokens, totalTokens, conversationRounds, startedAt, finishedAt, updatedAt int64
	err := db.QueryRowContext(ctx, `
		SELECT t.source_type, COALESCE(NULLIF(t.work_item_type,''), t.cloud_work_item_type, ''),
		       COALESCE(NULLIF(t.work_item_id,''), t.cloud_work_item_id, ''),
		       COALESCE(t.workspace_id, 0), t.title, t.status, t.execution_status,
		       t.current_step_uuid, COALESCE(t.cli_type,''), COALESCE(s.source_pipeline_id,''),
		       COALESCE(s.name,''),
		       COALESCE(t.input_tokens,0), COALESCE(t.output_tokens,0), COALESCE(t.total_tokens,0),
		       COALESCE(t.conversation_rounds,0), COALESCE(t.started_at,0), COALESCE(t.finished_at,0), t.updated_at
		 FROM gt_tasks t LEFT JOIN gt_task_pipeline_snapshots s ON s.uuid = t.pipeline_snapshot_uuid
		 WHERE t.uuid = ?`, taskUUID).Scan(
		&sourceType, &workItemType, &workItemID, &workspaceID, &title, &taskStatus, &execStatus,
		&currentStepUUID, &cliType, &pipelineID, &pipelineName,
		&inputTokens, &outputTokens, &totalTokens, &conversationRounds, &startedAt, &finishedAt, &updatedAt)
	if err == sql.ErrNoRows {
		applog.Warn("[CloudSync] 任务投影跳过推送：本地任务不存在（可能已被删除）", "task_uuid", taskUUID)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取任务主信息失败: %w", err)
	}
	if sourceType != "cloud" {
		// Only cloud-sourced tasks are mirrored to the cloud.
		applog.Info("[CloudSync] 任务投影跳过推送：仅 cloud 来源任务才镜像到云端", "task_uuid", taskUUID, "source_type", sourceType)
		return nil, nil
	}
	workItemIDInt, _ := strconv.ParseInt(workItemID, 10, 64)
	workspaceIDInt, _ := strconv.ParseInt(workspaceID, 10, 64)
	pipelineIDInt, _ := strconv.ParseInt(pipelineID, 10, 64)
	currentStepKey := ""
	if currentStepUUID != "" {
		_ = db.QueryRowContext(ctx,
			`SELECT step_key FROM gt_task_steps WHERE task_uuid = ? AND uuid = ?`, taskUUID, currentStepUUID).Scan(&currentStepKey)
	}
	base := &taskBase{
		workItem: cloud.PipelineTaskWorkItem{
			Type:        workItemType,
			ID:          workItemIDInt,
			WorkspaceID: workspaceIDInt,
			Title:       title,
		},
		pipeline: cloud.PipelineTaskPipeline{
			ID:              pipelineIDInt,
			Name:            pipelineName,
			Color:           "", // 本地快照无颜色字段；云端 pipeline_color_snapshot 为 VARCHAR(20)，不可传头像 URL 以免超长被拒
			CliType:         cliType,
			WorkflowVersion: 1,
		},
		taskStatus:         taskStatus,
		executionStatus:    execStatus,
		currentStepKey:     currentStepKey,
		executionSummary:   "",
		startedAt:          unixMillisToRFC3339(startedAt),
		finishedAt:         unixMillisToRFC3339(finishedAt),
		lastLocalUpdatedAt: time.UnixMilli(updatedAt).UTC().Format(time.RFC3339Nano),
		usage: cloud.PipelineTaskUsage{
			InputTokens:        inputTokens,
			OutputTokens:       outputTokens,
			TotalTokens:        totalTokens,
			ConversationRounds: int(conversationRounds),
		},
	}
	return base, nil
}

// ======================== Steps aggregation ========================

func loadTaskSteps(ctx context.Context, db *sql.DB, taskUUID string) ([]cloud.PipelineTaskStep, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT step_key, name, status, execution_status, prompt_snapshot, output_summary, error_summary,
		       input_tokens, output_tokens, total_tokens, conversation_rounds,
		       started_at, finished_at, duration_ms, step_order
		 FROM gt_task_steps WHERE task_uuid = ? ORDER BY step_order`, taskUUID)
	if err != nil {
		return nil, fmt.Errorf("查询任务步骤失败: %w", err)
	}
	defer rows.Close()
	steps := make([]cloud.PipelineTaskStep, 0)
	for rows.Next() {
		var stepKey, name, status, execStatus, promptSnapshot, outputSummary, errorSummary string
		var inputTokens, outputTokens, totalTokens, conversationRounds, startedAt, finishedAt, durationMs, sortOrder int64
		if err := rows.Scan(&stepKey, &name, &status, &execStatus, &promptSnapshot, &outputSummary, &errorSummary,
			&inputTokens, &outputTokens, &totalTokens, &conversationRounds,
			&startedAt, &finishedAt, &durationMs, &sortOrder); err != nil {
			return nil, fmt.Errorf("读取任务步骤失败: %w", err)
		}
		steps = append(steps, cloud.PipelineTaskStep{
			StepKey:       stepKey,
			StepName:      name,
			SortOrder:     int(sortOrder),
			Status:        cloudStepStatus(status, execStatus),
			PromptSummary: truncateRunes(strings.TrimSpace(promptSnapshot), 512),
			OutputSummary: outputSummary,
			ErrorSummary:  errorSummary,
			Usage: cloud.PipelineTaskUsage{
				InputTokens:        inputTokens,
				OutputTokens:       outputTokens,
				TotalTokens:        totalTokens,
				ConversationRounds: int(conversationRounds),
			},
			StartedAt:  unixMillisToRFC3339(startedAt),
			FinishedAt: unixMillisToRFC3339(finishedAt),
			DurationMs: durationMs,
		})
	}
	return steps, rows.Err()
}

// cloudStepStatus maps the local step lifecycle to the cloud step status vocabulary
// (pending/running/success/failed/stopped/idle).
func cloudStepStatus(status, execStatus string) string {
	if execStatus != "" && execStatus != "idle" {
		return execStatus
	}
	switch status {
	case "completed":
		return "success"
	case "active":
		return "running"
	case "pending":
		return "pending"
	default:
		return "idle"
	}
}

// ======================== Step conversation aggregation ========================

type roundRow struct {
	sessionUUID      string
	roundNo          int
	userMessage      string
	assistantMessage string
	status           string
	cliType          string
	errorSummary     string
	inputTokens      int64
	outputTokens     int64
	totalTokens      int64
	durationMs       int64
	createdAt        int64
	startedAt        int64
	finishedAt       int64
}

// stepConversationResult is the aggregated full conversation body of one step plus
// its usage/time rollups, carried by the step-level session sync payload.
type stepConversationResult struct {
	content      cloud.PipelineTaskConversationContent
	rounds       int
	inputTokens  int64
	outputTokens int64
	totalTokens  int64
	durationMs   int64
	startedAt    int64
	finishedAt   int64
}

// buildStepConversation loads every conversation round of one step (ordered by
// created_at) and aggregates them into the FULL step conversation body. Rounds are
// renumbered to be contiguous 1..N: the cloud upserts the conversation as a whole,
// so a truncated round list would overwrite earlier rounds and follow-up questions
// would never appear on the cloud.
func buildStepConversation(ctx context.Context, db *sql.DB, taskUUID, stepUUID string) (*stepConversationResult, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT cs.session_uuid, cs.round_no, cs.request_summary, cs.response_summary,
		       COALESCE(cs.token_input,0), COALESCE(cs.token_output,0), COALESCE(cs.token_total,0),
		       COALESCE(cs.duration_ms,0), cs.created_at,
		       COALESCE(s.status,''), COALESCE(s.cli_type,''), COALESCE(s.error_message,''),
		       COALESCE(s.started_at,0), COALESCE(s.finished_at,0)
		 FROM gt_conversation_summaries cs
		 JOIN gt_cli_sessions s ON s.uuid = cs.session_uuid
		 WHERE s.task_uuid = ? AND s.step_uuid = ? AND cs.created_at > 0
		 ORDER BY cs.created_at, cs.rowid`, taskUUID, stepUUID)
	if err != nil {
		return nil, fmt.Errorf("查询步骤会话摘要失败: %w", err)
	}
	defer rows.Close()
	rounds := make([]roundRow, 0)
	for rows.Next() {
		var r roundRow
		if err := rows.Scan(&r.sessionUUID, &r.roundNo, &r.userMessage, &r.assistantMessage,
			&r.inputTokens, &r.outputTokens, &r.totalTokens, &r.durationMs, &r.createdAt,
			&r.status, &r.cliType, &r.errorSummary, &r.startedAt, &r.finishedAt); err != nil {
			return nil, fmt.Errorf("读取步骤会话摘要失败: %w", err)
		}
		rounds = append(rounds, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := &stepConversationResult{
		content: cloud.PipelineTaskConversationContent{
			Rounds: make([]cloud.PipelineTaskRound, 0, len(rounds)),
		},
	}
	for index, r := range rounds {
		assistant := strings.TrimSpace(r.assistantMessage)
		if assistant == "" {
			assistant = assistantMessageContent(r)
		}
		result.content.Rounds = append(result.content.Rounds, cloud.PipelineTaskRound{
			RoundNo:     index + 1, // renumbered to be contiguous
			SessionUUID: r.sessionUUID,
			Status:      r.status,
			UserMessage: r.userMessage,
			AssistantMessages: []cloud.PipelineTaskMessage{{
				Sequence: 1,
				Role:     "assistant",
				Content:  assistant,
			}},
			ErrorSummary: r.errorSummary,
			Usage: cloud.PipelineTaskUsage{
				InputTokens:        r.inputTokens,
				OutputTokens:       r.outputTokens,
				TotalTokens:        r.totalTokens,
				ConversationRounds: 1,
			},
			StartedAt:  unixMillisToRFC3339(r.startedAt),
			FinishedAt: unixMillisToRFC3339(r.finishedAt),
			DurationMs: r.durationMs,
		})
		result.inputTokens += r.inputTokens
		result.outputTokens += r.outputTokens
		result.totalTokens += r.totalTokens
		result.durationMs += r.durationMs
		result.startedAt = minNonZero(result.startedAt, r.startedAt)
		if r.finishedAt > result.finishedAt {
			result.finishedAt = r.finishedAt
		}
	}
	result.content.RoundCount = len(result.content.Rounds)
	result.rounds = len(result.content.Rounds)
	return result, nil
}

// assistantMessageContent returns the round's visible AI reply, falling back to the
// error summary or a status placeholder: the cloud rejects empty assistant messages,
// and a single empty row would permanently block the whole task's sync.
func assistantMessageContent(r roundRow) string {
	if content := strings.TrimSpace(r.assistantMessage); content != "" {
		return r.assistantMessage
	}
	if content := strings.TrimSpace(r.errorSummary); content != "" {
		return content
	}
	return "CLI 执行" + r.status
}

// ======================== Helpers ========================

func projectionHash(base *taskBase, steps []cloud.PipelineTaskStep) string {
	hasher := sha256.New()
	hasher.Write([]byte(base.taskStatus))
	hasher.Write([]byte(base.executionStatus))
	hasher.Write([]byte(base.currentStepKey))
	hasher.Write([]byte(base.lastLocalUpdatedAt))
	for _, step := range steps {
		hasher.Write([]byte(step.StepKey + step.Status + step.OutputSummary + step.ErrorSummary))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// unixMillisToRFC3339 converts local Unix-millisecond timestamps to cloud RFC3339 strings.
func unixMillisToRFC3339(ms int64) *string {
	if ms <= 0 {
		return nil
	}
	value := time.UnixMilli(ms).UTC().Format(time.RFC3339)
	return &value
}

func parseMillis(rfc3339 string) int64 {
	if rfc3339 == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, rfc3339)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func minNonZero(current, candidate int64) int64 {
	if candidate <= 0 {
		return current
	}
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}
