package taskruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const executionHistoryPayloadVersion = 1

var ErrExecutionHistoryNotFound = errors.New("历史执行不存在")

type ExecutionHistorySummary struct {
	UUID          string `json:"uuid"`
	TaskUUID      string `json:"task_uuid"`
	ExecutionNo   int    `json:"execution_no"`
	ExecutionMode string `json:"execution_mode"`
	ExecutionTool string `json:"execution_tool"`
	TargetName    string `json:"target_name"`
	ModelName     string `json:"model_name"`
	StartedAt     int64  `json:"started_at"`
	ArchivedAt    int64  `json:"archived_at"`
}

type ExecutionHistoryStep struct {
	UUID            string `json:"uuid"`
	StepKey         string `json:"step_key"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Avatar          string `json:"avatar"`
	SortOrder       int    `json:"sort_order"`
	CLIType         string `json:"cli_type"`
	ModelName       string `json:"model_name"`
	Status          string `json:"status"`
	ExecutionStatus string `json:"execution_status"`
	PromptSnapshot  string `json:"prompt_snapshot"`
	StepDir         string `json:"step_dir"`
	MemberRole      string `json:"member_role"`
	CreatedAt       int64  `json:"created_at"`
}

type ExecutionHistoryProgress struct {
	UUID               string `json:"uuid"`
	TaskUUID           string `json:"task_uuid"`
	TaskStepUUID       string `json:"task_step_uuid"`
	SessionUUID        string `json:"session_uuid"`
	RecordType         string `json:"record_type"`
	UserPrompt         string `json:"user_prompt"`
	CLIType            string `json:"cli_type"`
	ModelName          string `json:"model"`
	Status             string `json:"status"`
	FinalResult        string `json:"final_result"`
	ExecutionMode      string `json:"execution_mode"`
	ExternalSessionID  string `json:"external_session_id"`
	DispatchStatus     string `json:"dispatch_status"`
	DispatchMessage    string `json:"dispatch_message"`
	CreatedAt          int64  `json:"created_at"`
	StartedAt          int64  `json:"started_at"`
	FinishedAt         int64  `json:"finished_at"`
	LatestEventType    string `json:"latest_event_type"`
	LatestEventContent string `json:"latest_event_content"`
	LatestEventAt      int64  `json:"latest_event_at"`
	InputTokens        int64  `json:"input_tokens"`
	OutputTokens       int64  `json:"output_tokens"`
	TotalTokens        int64  `json:"total_tokens"`
	DurationMs         int64  `json:"duration_ms"`
}

type ExecutionHistorySession struct {
	UUID              string `json:"uuid"`
	StepUUID          string `json:"step_uuid"`
	ConversationUUID  string `json:"conversation_uuid"`
	ParentSessionUUID string `json:"parent_session_uuid"`
	RunNo             int    `json:"run_no"`
	Status            string `json:"status"`
	CLIType           string `json:"cli_type"`
	ModelName         string `json:"model_name"`
	ExternalSessionID string `json:"external_session_id"`
	ResultStatus      string `json:"result_status"`
	FinalResult       string `json:"final_result"`
	ErrorCode         string `json:"error_code"`
	ErrorMessage      string `json:"error_message"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	StartedAt         int64  `json:"started_at"`
	FinishedAt        int64  `json:"finished_at"`
	DurationMs        int64  `json:"duration_ms"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

type ExecutionHistoryEvent struct {
	SessionUUID string          `json:"session_uuid"`
	Sequence    int             `json:"sequence"`
	EventType   string          `json:"event_type"`
	Event       json.RawMessage `json:"event,omitempty"`
	CreatedAt   int64           `json:"created_at"`
}

type ExecutionHistoryConversationSummary struct {
	UUID             string `json:"uuid"`
	ConversationUUID string `json:"conversation_uuid"`
	SessionUUID      string `json:"session_uuid"`
	StepKey          string `json:"step_key"`
	RoundNo          int    `json:"round_no"`
	RequestSummary   string `json:"request_summary"`
	ResponseSummary  string `json:"response_summary"`
	ModelName        string `json:"model_name"`
	TokenInput       int64  `json:"token_input"`
	TokenOutput      int64  `json:"token_output"`
	TokenTotal       int64  `json:"token_total"`
	DurationMs       int64  `json:"duration_ms"`
	CreatedAt        int64  `json:"created_at"`
}

type ExecutionHistoryPayload struct {
	Version               int                                   `json:"version"`
	Steps                 []ExecutionHistoryStep                `json:"steps"`
	Progress              []ExecutionHistoryProgress            `json:"progress"`
	Sessions              []ExecutionHistorySession             `json:"sessions"`
	SessionEvents         map[string][]ExecutionHistoryEvent    `json:"session_events"`
	ConversationSummaries []ExecutionHistoryConversationSummary `json:"conversation_summaries"`
}

type ExecutionHistoryDetail struct {
	ExecutionHistorySummary
	ExecutionHistoryPayload
}

func (s *Service) ListExecutionHistory(ctx context.Context, taskUUID string) ([]ExecutionHistorySummary, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(&exists); err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT uuid, task_uuid, execution_no, execution_mode, execution_tool,
		target_name, model_name, started_at, archived_at
		FROM gt_task_execution_history WHERE task_uuid=? ORDER BY execution_no`, taskUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ExecutionHistorySummary, 0)
	for rows.Next() {
		var item ExecutionHistorySummary
		if err := rows.Scan(&item.UUID, &item.TaskUUID, &item.ExecutionNo, &item.ExecutionMode,
			&item.ExecutionTool, &item.TargetName, &item.ModelName, &item.StartedAt, &item.ArchivedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetExecutionHistory(ctx context.Context, taskUUID, historyUUID string) (ExecutionHistoryDetail, error) {
	var detail ExecutionHistoryDetail
	var payloadJSON string
	err := s.db.QueryRowContext(ctx, `SELECT uuid, task_uuid, execution_no, execution_mode, execution_tool,
		target_name, model_name, started_at, archived_at, payload_json
		FROM gt_task_execution_history WHERE task_uuid=? AND uuid=?`, taskUUID, historyUUID).Scan(
		&detail.UUID, &detail.TaskUUID, &detail.ExecutionNo, &detail.ExecutionMode,
		&detail.ExecutionTool, &detail.TargetName, &detail.ModelName, &detail.StartedAt,
		&detail.ArchivedAt, &payloadJSON)
	if err == sql.ErrNoRows {
		return detail, ErrExecutionHistoryNotFound
	}
	if err != nil {
		return detail, err
	}
	if err := json.Unmarshal([]byte(payloadJSON), &detail.ExecutionHistoryPayload); err != nil {
		return detail, fmt.Errorf("解析历史执行快照失败: %w", err)
	}
	if detail.Version != executionHistoryPayloadVersion {
		return detail, fmt.Errorf("不支持的历史执行快照版本: %d", detail.Version)
	}
	return detail, nil
}

func archiveCurrentExecutionTx(ctx context.Context, tx *sql.Tx, taskUUID string) error {
	payload := ExecutionHistoryPayload{
		Version:               executionHistoryPayloadVersion,
		Steps:                 make([]ExecutionHistoryStep, 0),
		Progress:              make([]ExecutionHistoryProgress, 0),
		Sessions:              make([]ExecutionHistorySession, 0),
		SessionEvents:         make(map[string][]ExecutionHistoryEvent),
		ConversationSummaries: make([]ExecutionHistoryConversationSummary, 0),
	}

	var summary ExecutionHistorySummary
	var taskStartedAt, taskUpdatedAt, snapshotCreatedAt int64
	err := tx.QueryRowContext(ctx, `SELECT t.execution_mode, t.execution_tool, t.started_at, t.updated_at,
		COALESCE(NULLIF(ps.name,''), NULLIF(p.name,''), NULLIF(es.name,''), NULLIF(eg.name,''), ''),
		COALESCE(ps.created_at, es.created_at, 0)
		FROM gt_tasks t
		LEFT JOIN gt_task_pipeline_snapshots ps ON ps.uuid=t.pipeline_snapshot_uuid
		LEFT JOIN gt_pipelines p ON p.uuid=t.selected_pipeline_uuid
		LEFT JOIN gt_task_expert_group_snapshots es ON es.uuid=t.expert_group_snapshot_uuid
		LEFT JOIN gt_expert_groups eg ON eg.uuid=t.selected_expert_group_uuid
		WHERE t.uuid=?`, taskUUID).Scan(&summary.ExecutionMode, &summary.ExecutionTool,
		&taskStartedAt, &taskUpdatedAt, &summary.TargetName, &snapshotCreatedAt)
	if err != nil {
		return err
	}
	if summary.ExecutionMode == ExecutionModeCLI {
		_ = tx.QueryRowContext(ctx, `SELECT model_name FROM gt_task_steps WHERE task_uuid=? AND step_key=? LIMIT 1`,
			taskUUID, CLIDirectStepKey).Scan(&summary.ModelName)
	}
	if summary.TargetName == "" && (summary.ExecutionMode == ExecutionModeCLI || summary.ExecutionMode == ExecutionModeVibeCoding) {
		summary.TargetName = summary.ExecutionTool
	}

	stepRows, err := tx.QueryContext(ctx, `SELECT uuid, step_key, name, description, avatar, step_order,
		cli_type, model_name, status, execution_status, prompt_snapshot, step_dir, member_role, created_at
		FROM gt_task_steps WHERE task_uuid=? ORDER BY step_order`, taskUUID)
	if err != nil {
		return err
	}
	for stepRows.Next() {
		var item ExecutionHistoryStep
		if err := stepRows.Scan(&item.UUID, &item.StepKey, &item.Name, &item.Description, &item.Avatar,
			&item.SortOrder, &item.CLIType, &item.ModelName, &item.Status, &item.ExecutionStatus,
			&item.PromptSnapshot, &item.StepDir, &item.MemberRole, &item.CreatedAt); err != nil {
			stepRows.Close()
			return err
		}
		payload.Steps = append(payload.Steps, item)
	}
	if err := stepRows.Err(); err != nil {
		stepRows.Close()
		return err
	}
	if err := stepRows.Close(); err != nil {
		return err
	}

	progressRows, err := tx.QueryContext(ctx, `SELECT p.uuid, p.task_uuid, p.task_step_uuid, p.session_uuid,
		p.record_type, p.user_prompt, p.cli_type, p.model_name, p.status, p.final_result,
		p.execution_mode, p.external_session_id, p.dispatch_status, p.dispatch_message,
		p.created_at, p.started_at, p.finished_at,
		COALESCE(s.latest_event_type,''), COALESCE(s.latest_event_content,''), COALESCE(s.latest_event_at,0),
		COALESCE(s.input_tokens,0), COALESCE(s.output_tokens,0), COALESCE(s.total_tokens,0), COALESCE(s.duration_ms,0)
		FROM gt_task_progress p LEFT JOIN gt_cli_sessions s ON s.uuid=p.session_uuid
		WHERE p.task_uuid=? ORDER BY p.created_at, p.rowid`, taskUUID)
	if err != nil {
		return err
	}
	for progressRows.Next() {
		var item ExecutionHistoryProgress
		if err := progressRows.Scan(&item.UUID, &item.TaskUUID, &item.TaskStepUUID, &item.SessionUUID,
			&item.RecordType, &item.UserPrompt, &item.CLIType, &item.ModelName, &item.Status,
			&item.FinalResult, &item.ExecutionMode, &item.ExternalSessionID, &item.DispatchStatus,
			&item.DispatchMessage, &item.CreatedAt, &item.StartedAt, &item.FinishedAt,
			&item.LatestEventType, &item.LatestEventContent, &item.LatestEventAt,
			&item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.DurationMs); err != nil {
			progressRows.Close()
			return err
		}
		payload.Progress = append(payload.Progress, item)
	}
	if err := progressRows.Err(); err != nil {
		progressRows.Close()
		return err
	}
	if err := progressRows.Close(); err != nil {
		return err
	}

	sessionRows, err := tx.QueryContext(ctx, `SELECT uuid, step_uuid, conversation_uuid, parent_session_uuid, run_no,
		status, cli_type, model_name, external_session_id, result_status, final_result, error_code, error_message,
		input_tokens, output_tokens, total_tokens, started_at, finished_at, duration_ms, created_at, updated_at
		FROM gt_cli_sessions WHERE task_uuid=? ORDER BY created_at, rowid`, taskUUID)
	if err != nil {
		return err
	}
	for sessionRows.Next() {
		var item ExecutionHistorySession
		if err := sessionRows.Scan(&item.UUID, &item.StepUUID, &item.ConversationUUID,
			&item.ParentSessionUUID, &item.RunNo, &item.Status, &item.CLIType, &item.ModelName,
			&item.ExternalSessionID, &item.ResultStatus, &item.FinalResult, &item.ErrorCode,
			&item.ErrorMessage, &item.InputTokens, &item.OutputTokens, &item.TotalTokens,
			&item.StartedAt, &item.FinishedAt, &item.DurationMs, &item.CreatedAt, &item.UpdatedAt); err != nil {
			sessionRows.Close()
			return err
		}
		payload.Sessions = append(payload.Sessions, item)
	}
	if err := sessionRows.Err(); err != nil {
		sessionRows.Close()
		return err
	}
	if err := sessionRows.Close(); err != nil {
		return err
	}

	eventRows, err := tx.QueryContext(ctx, `SELECT e.session_uuid, e.sequence, e.event_type, e.payload_json, e.created_at
		FROM gt_session_events e JOIN gt_cli_sessions s ON s.uuid=e.session_uuid
		WHERE s.task_uuid=? ORDER BY e.session_uuid, e.sequence`, taskUUID)
	if err != nil {
		return err
	}
	for eventRows.Next() {
		var item ExecutionHistoryEvent
		var raw string
		if err := eventRows.Scan(&item.SessionUUID, &item.Sequence, &item.EventType, &raw, &item.CreatedAt); err != nil {
			eventRows.Close()
			return err
		}
		if json.Valid([]byte(raw)) {
			item.Event = json.RawMessage(raw)
		}
		payload.SessionEvents[item.SessionUUID] = append(payload.SessionEvents[item.SessionUUID], item)
	}
	if err := eventRows.Err(); err != nil {
		eventRows.Close()
		return err
	}
	if err := eventRows.Close(); err != nil {
		return err
	}

	summaryRows, err := tx.QueryContext(ctx, `SELECT cs.uuid, cs.conversation_uuid, cs.session_uuid, cs.step_key,
		cs.round_no, cs.request_summary, cs.response_summary, cs.model_name, cs.token_input, cs.token_output,
		cs.token_total, cs.duration_ms, cs.created_at
		FROM gt_conversation_summaries cs JOIN gt_cli_sessions s ON s.uuid=cs.session_uuid
		WHERE s.task_uuid=? ORDER BY cs.created_at, cs.rowid`, taskUUID)
	if err != nil {
		return err
	}
	for summaryRows.Next() {
		var item ExecutionHistoryConversationSummary
		if err := summaryRows.Scan(&item.UUID, &item.ConversationUUID, &item.SessionUUID, &item.StepKey,
			&item.RoundNo, &item.RequestSummary, &item.ResponseSummary, &item.ModelName,
			&item.TokenInput, &item.TokenOutput, &item.TokenTotal, &item.DurationMs, &item.CreatedAt); err != nil {
			summaryRows.Close()
			return err
		}
		payload.ConversationSummaries = append(payload.ConversationSummaries, item)
	}
	if err := summaryRows.Err(); err != nil {
		summaryRows.Close()
		return err
	}
	if err := summaryRows.Close(); err != nil {
		return err
	}

	var earliestProgress, earliestStep int64
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(created_at),0) FROM gt_task_progress WHERE task_uuid=?`, taskUUID).Scan(&earliestProgress)
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(created_at),0) FROM gt_task_steps WHERE task_uuid=?`, taskUUID).Scan(&earliestStep)
	summary.StartedAt = firstPositive(taskStartedAt, earliestProgress, earliestStep, snapshotCreatedAt, taskUpdatedAt)
	summary.UUID = uuid.NewString()
	summary.TaskUUID = taskUUID
	summary.ArchivedAt = time.Now().UnixMilli()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(execution_no),0)+1 FROM gt_task_execution_history WHERE task_uuid=?`, taskUUID).Scan(&summary.ExecutionNo); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gt_task_execution_history
		(uuid, task_uuid, execution_no, execution_mode, execution_tool, target_name, model_name, started_at, archived_at, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, summary.UUID, summary.TaskUUID, summary.ExecutionNo,
		summary.ExecutionMode, summary.ExecutionTool, strings.TrimSpace(summary.TargetName), strings.TrimSpace(summary.ModelName),
		summary.StartedAt, summary.ArchivedAt, string(payloadJSON))
	return err
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
