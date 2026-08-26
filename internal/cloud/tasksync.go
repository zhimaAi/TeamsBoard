package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"goteams-client/internal/applog"
)

// PipelineTaskWorkItem is the snapshot of the work item linked to the task.
type PipelineTaskWorkItem struct {
	Type        string `json:"type"`
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspace_id"`
	Title       string `json:"title"`
}

// PipelineTaskPipeline is the snapshot of the pipeline used by the task.
type PipelineTaskPipeline struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	CliType         string `json:"cli_type"`
	WorkflowVersion int    `json:"workflow_version"`
}

// PipelineTaskUsage holds token and conversation-round statistics.
type PipelineTaskUsage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	TotalTokens        int64 `json:"total_tokens"`
	ConversationRounds int   `json:"conversation_rounds"`
}

// PipelineTaskStep is the task step snapshot pushed to the cloud.
type PipelineTaskStep struct {
	StepKey       string            `json:"step_key"`
	StepName      string            `json:"step_name"`
	SortOrder     int               `json:"sort_order"`
	Status        string            `json:"status"`
	PromptSummary string            `json:"prompt_summary"`
	OutputSummary string            `json:"output_summary"`
	ErrorSummary  string            `json:"error_summary"`
	Usage         PipelineTaskUsage `json:"usage"`
	StartedAt     *string           `json:"started_at"`
	FinishedAt    *string           `json:"finished_at"`
	DurationMs    int64             `json:"duration_ms"`
}

// PipelineTaskMessage is a single AI-visible message pushed to the cloud.
type PipelineTaskMessage struct {
	Sequence  int    `json:"sequence"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// PipelineTaskRound is one user question and AI reply within a step conversation.
type PipelineTaskRound struct {
	RoundNo           int                   `json:"round_no"`
	SessionUUID       string                `json:"session_uuid"`
	Status            string                `json:"status"`
	UserMessage       string                `json:"user_message"`
	AssistantMessages []PipelineTaskMessage `json:"assistant_messages"`
	ErrorSummary      string                `json:"error_summary"`
	Usage             PipelineTaskUsage     `json:"usage"`
	StartedAt         *string               `json:"started_at"`
	FinishedAt        *string               `json:"finished_at"`
	DurationMs        int64                 `json:"duration_ms"`
}

// PipelineTaskConversationContent holds the full step conversation body pushed to the cloud.
type PipelineTaskConversationContent struct {
	RoundCount int                 `json:"round_count"`
	Rounds     []PipelineTaskRound `json:"rounds"`
}

// PipelineTaskSync is the incremental task sync payload pushed to the cloud via REST.
// Steps are incremental payloads upserted by the cloud; the deleted step list lets the
// client explicitly remove cloud rows that no longer exist locally. Conversation content
// is NOT part of this channel anymore: each step's full conversation is pushed via the
// step-level session sync (PushPipelineTaskSession).
type PipelineTaskSync struct {
	LocalTaskUUID      string               `json:"local_task_uuid"`
	LocalRevision      int64                `json:"local_revision"`
	ProjectionHash     string               `json:"projection_hash"`
	WorkItem           PipelineTaskWorkItem `json:"work_item"`
	Pipeline           PipelineTaskPipeline `json:"pipeline"`
	TaskStatus         string               `json:"task_status"`
	ExecutionStatus    string               `json:"execution_status"`
	CurrentStepKey     string               `json:"current_step_key"`
	ExecutionSummary   string               `json:"execution_summary"`
	StartedAt          *string              `json:"started_at"`
	FinishedAt         *string              `json:"finished_at"`
	LastLocalUpdatedAt string               `json:"last_local_updated_at"`
	Usage              PipelineTaskUsage    `json:"usage"`
	Steps              []PipelineTaskStep   `json:"steps"`
	DeletedStepKeys    []string             `json:"deleted_step_keys"`
}

// PipelineTaskSyncResult is the cloud response for an incremental task sync push.
type PipelineTaskSyncResult struct {
	CloudTaskID    int64  `json:"cloud_task_id"`
	LocalRevision  int64  `json:"local_revision"`
	ProjectionHash string `json:"projection_hash"`
	LastUploadedAt int64  `json:"last_uploaded_at"`
	Result         string `json:"result"`
}

// PushPipelineTaskSync pushes the incremental task sync payload to the cloud (REST, replaces the removed WebSocket channel).
func (c *Client) PushPipelineTaskSync(ctx context.Context, payload PipelineTaskSync) (*PipelineTaskSyncResult, error) {
	var result PipelineTaskSyncResult
	if err := c.do(ctx, http.MethodPost, "/api/client/pipeline-tasks/sync", payload, &result); err != nil {
		applog.Error("[CloudSync] 推送任务投影失败", "云端地址", c.BaseURL(), "task_uuid", payload.LocalTaskUUID, "error", err.Error())
		return nil, err
	}
	return &result, nil
}

// DeletePipelineTask deletes the cloud task projection by the local task uuid.
func (c *Client) DeletePipelineTask(ctx context.Context, localTaskUUID string) error {
	path := "/api/client/pipeline-tasks/" + url.PathEscape(localTaskUUID)
	var result struct {
		Result string `json:"result"`
	}
	if err := c.do(ctx, http.MethodDelete, path, nil, &result); err != nil {
		return err
	}
	if result.Result != "deleted" {
		return fmt.Errorf("云端任务删除未确认: %s", result.Result)
	}
	return nil
}
