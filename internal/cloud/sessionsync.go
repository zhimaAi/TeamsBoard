package cloud

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"goteams-client/internal/applog"
)

// PipelineTaskSessionSync is the step-level session projection pushed to the cloud:
// one row per step carrying the step's full conversation JSON and the aggregated run
// result. The cloud upserts by (admin_id, pipeline_task_id, step_key) and guards
// against out-of-order pushes via client_updated_at.
type PipelineTaskSessionSync struct {
	LocalTaskUUID      string                          `json:"local_task_uuid"`
	StepKey            string                          `json:"step_key"`
	StepName           string                          `json:"step_name"`
	Status             string                          `json:"status"`
	CliType            string                          `json:"cli_type"`
	ModelName          string                          `json:"model_name"`
	PromptSnapshot     string                          `json:"prompt_snapshot"`
	ResultStatus       string                          `json:"result_status"`
	FinalResult        string                          `json:"final_result"`
	ExitCode           int                             `json:"exit_code"`
	ErrorCode          string                          `json:"error_code"`
	ErrorMessage       string                          `json:"error_message"`
	InputTokens        int64                           `json:"input_tokens"`
	OutputTokens       int64                           `json:"output_tokens"`
	TotalTokens        int64                           `json:"total_tokens"`
	ConversationRounds int                             `json:"conversation_rounds"`
	Conversation       PipelineTaskConversationContent `json:"conversation"`
	StartedAt          int64                           `json:"started_at"`
	FinishedAt         int64                           `json:"finished_at"`
	DurationMs         int64                           `json:"duration_ms"`
	CreatedAt          int64                           `json:"created_at"`
	ClientUpdatedAt    int64                           `json:"client_updated_at"`
}

// PushPipelineTaskSession pushes the step-level session projection to the cloud.
// Sync success is defined purely by the HTTP transport result (2xx): the business
// result inside the response body is intentionally ignored, so any non-2xx status
// (e.g. 502) is returned as an error carrying the HTTP status code.
func (c *Client) PushPipelineTaskSession(ctx context.Context, payload PipelineTaskSessionSync) error {
	resp, err := c.doRaw(ctx, http.MethodPost, "/api/client/pipeline-tasks/sessions", payload, nil)
	if err != nil {
		applog.Error("[CloudSync] 推送步骤会话失败", "云端地址", c.BaseURL(), "task_uuid", payload.LocalTaskUUID, "step_key", payload.StepKey, "error", err.Error())
		return fmt.Errorf("请求云端失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseAPIError(resp)
		applog.Error("[CloudSync] 推送步骤会话被云端拒绝",
			"云端地址", c.BaseURL(), "task_uuid", payload.LocalTaskUUID, "step_key", payload.StepKey,
			"status_code", resp.StatusCode, "error", apiErr.Error())
		return apiErr
	}
	// Drain the body so the underlying connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	return nil
}
