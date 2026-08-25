package cloudsync

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
)

// SessionSyncService is a standalone asynchronous service that pushes cloud task
// step conversations to the cloud. Whenever a gt_cli_sessions row of a cloud task
// changes, its step becomes pending and is re-pushed with the step's FULL conversation
// JSON plus the latest run result. Per task the pending steps are pushed strictly in
// step creation order: if the earliest pending step fails to push, later steps of the
// same task are held back so the cloud never sees an out-of-order step history.
//
// Success semantics: an HTTP 2xx response counts as sync success regardless of the
// business result in the response body; any transport failure or non-2xx status is
// recorded on the step's session rows as the failure reason (e.g. HTTP 502).
type SessionSyncService struct {
	db       *sql.DB
	client   func() *cloud.Client
	interval time.Duration
}

// syncStatus values stored in gt_cli_sessions.cloud_sync_status.
const (
	syncStatusSuccess = "success"
	syncStatusFailed  = "failed"
)

// maxStepPushesPerTaskPerRound bounds how many step sessions one task may push in a
// single round, protecting the loop from pathological backlogs.
const maxStepPushesPerTaskPerRound = 200

// syncErrorMaxRunes bounds the failure reason written back to the session rows.
const syncErrorMaxRunes = 256

// NewSessionSyncService creates the session sync service. The client getter returns
// nil while no cloud account session is logged in, in which case rounds are skipped.
func NewSessionSyncService(db *sql.DB, client func() *cloud.Client) *SessionSyncService {
	return &SessionSyncService{db: db, client: client, interval: 10 * time.Second}
}

// Start launches the periodic sync loop; it stops when ctx is canceled.
func (s *SessionSyncService) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			s.syncRound(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// syncRound pushes pending step sessions once across all cloud tasks.
func (s *SessionSyncService) syncRound(ctx context.Context) {
	if s.db == nil || s.client == nil {
		return
	}
	client := s.client()
	if client == nil {
		return // not logged in to any cloud account
	}
	taskUUIDs, err := s.pendingTaskUUIDs(ctx)
	if err != nil {
		applog.Warn("[CloudSync] 查询待同步会话任务失败", "error", err.Error())
		return
	}
	for _, taskUUID := range taskUUIDs {
		if ctx.Err() != nil {
			return
		}
		s.syncTask(ctx, client, taskUUID)
	}
}

// pendingTaskUUIDs lists cloud tasks that have at least one session record that was
// never synced successfully or changed after its last successful sync.
func (s *SessionSyncService) pendingTaskUUIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.task_uuid
		 FROM gt_cli_sessions s
		 JOIN gt_tasks t ON t.uuid = s.task_uuid
		 WHERE t.source_type = 'cloud'
		   AND (s.cloud_sync_status <> ? OR s.updated_at > s.cloud_synced_at)
		 GROUP BY s.task_uuid
		 ORDER BY MIN(s.created_at) ASC, s.task_uuid ASC`, syncStatusSuccess)
	if err != nil {
		return nil, fmt.Errorf("查询待同步任务失败: %w", err)
	}
	defer rows.Close()
	taskUUIDs := make([]string, 0)
	for rows.Next() {
		var taskUUID string
		if err := rows.Scan(&taskUUID); err != nil {
			return nil, fmt.Errorf("读取待同步任务失败: %w", err)
		}
		taskUUIDs = append(taskUUIDs, taskUUID)
	}
	return taskUUIDs, rows.Err()
}

// syncTask pushes the task's pending steps in creation order. It stops at the first
// failure so ordering is preserved; the failed step is retried next round.
func (s *SessionSyncService) syncTask(ctx context.Context, client *cloud.Client, taskUUID string) {
	// 云端要求任务投影（task sync）必须先于会话记录（session sync）到达：若投影缺失，
	// 会话推送会被 404 拒绝并进入无限重试。这里在推送会话前先同步推送一次任务投影，
	// 保证顺序正确；云端 upsert 幂等，重复推送无害。当任务尚未分配流水线快照时，
	// BuildTaskSync 会静默跳过（见内部 WARN 日志），此时会话仍会 404——根因是任务未分配流水线。
	if err := defaultPusher.PushTaskSync(ctx, s.db, client, taskUUID); err != nil {
		applog.Warn("[CloudSync] 推送会话前同步任务投影失败", "task_uuid", taskUUID, "error", err.Error())
	}
	for pushed := 0; pushed < maxStepPushesPerTaskPerRound; pushed++ {
		if ctx.Err() != nil {
			return
		}
		payload, stepUUID, watermark, err := s.nextPendingStep(ctx, taskUUID)
		if err != nil {
			applog.Warn("[CloudSync] 读取待同步步骤会话失败", "task_uuid", taskUUID, "error", err.Error())
			return
		}
		if payload == nil {
			return // task fully synced
		}

		pushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = client.PushPipelineTaskSession(pushCtx, *payload)
		cancel()
		if err != nil {
			// Record the failure reason (HTTP status code such as 502, or transport error)
			// on the step's session rows and keep the step pending so the next round retries
			// it in the same order.
			s.markStepFailed(ctx, taskUUID, stepUUID, err.Error())
			applog.Warn("[CloudSync] 同步步骤会话失败", "task_uuid", taskUUID, "step_key", payload.StepKey, "error", err.Error())
			return
		}
		s.markStepSuccess(ctx, taskUUID, stepUUID, watermark)
	}
}

// nextPendingStep loads the task's earliest pending step and builds its step-level
// session payload. The returned watermark is the step's max session updated_at at
// read time: it becomes the sync cursor on success, so changes written during the
// push remain pending.
func (s *SessionSyncService) nextPendingStep(ctx context.Context, taskUUID string) (*cloud.PipelineTaskSessionSync, string, int64, error) {
	var stepUUID string
	err := s.db.QueryRowContext(ctx, `
		SELECT s.step_uuid
		 FROM gt_cli_sessions s
		 WHERE s.task_uuid = ?
		   AND (s.cloud_sync_status <> ? OR s.updated_at > s.cloud_synced_at)
		   AND s.step_uuid <> ''
		 GROUP BY s.step_uuid
		 ORDER BY MIN(s.created_at) ASC, MIN(s.rowid) ASC
		 LIMIT 1`, taskUUID, syncStatusSuccess).Scan(&stepUUID)
	if err == sql.ErrNoRows {
		return nil, "", 0, nil
	}
	if err != nil {
		return nil, "", 0, fmt.Errorf("查询最早待同步步骤失败: %w", err)
	}

	payload, watermark, err := buildStepSessionPayload(ctx, s.db, taskUUID, stepUUID)
	if err != nil {
		return nil, "", 0, err
	}
	return payload, stepUUID, watermark, nil
}

// buildStepSessionPayload aggregates one step's latest run metadata with its full
// conversation body into the step-level session sync payload.
func buildStepSessionPayload(ctx context.Context, db *sql.DB, taskUUID, stepUUID string) (*cloud.PipelineTaskSessionSync, int64, error) {
	var stepKey, stepName string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(step_key,''), COALESCE(name,'') FROM gt_task_steps WHERE uuid = ? AND task_uuid = ?`,
		stepUUID, taskUUID).Scan(&stepKey, &stepName)
	if err != nil {
		return nil, 0, fmt.Errorf("读取步骤信息失败: %w", err)
	}

	// Latest run row: status/model/prompt/result/error of the most recently updated run.
	var status, cliType, modelName, promptSnapshot, resultStatus, finalResult, errorCode, errorMessage string
	var exitCode int
	runErr := db.QueryRowContext(ctx, `
		SELECT COALESCE(s.status,''), COALESCE(s.cli_type,''), COALESCE(s.model_name,''),
		       COALESCE(s.prompt_snapshot,''), COALESCE(s.result_status,''), COALESCE(s.final_result,''),
		       COALESCE(s.exit_code,0), COALESCE(s.error_code,''), COALESCE(s.error_message,'')
		 FROM gt_cli_sessions s
		 WHERE s.task_uuid = ? AND s.step_uuid = ?
		 ORDER BY s.updated_at DESC, s.run_no DESC, s.rowid DESC
		 LIMIT 1`, taskUUID, stepUUID).Scan(
		&status, &cliType, &modelName, &promptSnapshot, &resultStatus, &finalResult,
		&exitCode, &errorCode, &errorMessage)
	if runErr != nil && runErr != sql.ErrNoRows {
		return nil, 0, fmt.Errorf("读取最近运行信息失败: %w", runErr)
	}

	// Whole-step rollups and the sync watermark (max session updated_at).
	var startedAt, finishedAt, durationMs, maxUpdatedAt, minCreatedAt int64
	aggErr := db.QueryRowContext(ctx, `
		SELECT COALESCE(MIN(NULLIF(s.started_at,0)),0), COALESCE(MAX(COALESCE(s.finished_at,0)),0),
		       COALESCE(SUM(COALESCE(s.duration_ms,0)),0), COALESCE(MAX(s.updated_at),0),
		       COALESCE(MIN(s.created_at),0)
		 FROM gt_cli_sessions s
		 WHERE s.task_uuid = ? AND s.step_uuid = ?`, taskUUID, stepUUID).Scan(
		&startedAt, &finishedAt, &durationMs, &maxUpdatedAt, &minCreatedAt)
	if aggErr != nil {
		return nil, 0, fmt.Errorf("读取步骤会话汇总失败: %w", aggErr)
	}

	conv, err := buildStepConversation(ctx, db, taskUUID, stepUUID)
	if err != nil {
		return nil, 0, err
	}
	payload := &cloud.PipelineTaskSessionSync{
		LocalTaskUUID:      taskUUID,
		StepKey:            stepKey,
		StepName:           stepName,
		Status:             status,
		CliType:            cliType,
		ModelName:          modelName,
		PromptSnapshot:     promptSnapshot,
		ResultStatus:       resultStatus,
		FinalResult:        finalResult,
		ExitCode:           exitCode,
		ErrorCode:          errorCode,
		ErrorMessage:       errorMessage,
		InputTokens:        conv.inputTokens,
		OutputTokens:       conv.outputTokens,
		TotalTokens:        conv.totalTokens,
		ConversationRounds: conv.rounds,
		Conversation:       conv.content,
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
		DurationMs:         durationMs,
		CreatedAt:          minCreatedAt,
		ClientUpdatedAt:    maxUpdatedAt,
	}
	return payload, maxUpdatedAt, nil
}

// markStepSuccess records a successful push for every session row of the step whose
// updated_at is not newer than the watermark; later changes become pending again.
func (s *SessionSyncService) markStepSuccess(ctx context.Context, taskUUID, stepUUID string, watermark int64) {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE gt_cli_sessions
		 SET cloud_sync_status = ?, cloud_sync_error = '', cloud_synced_at = updated_at
		 WHERE task_uuid = ? AND step_uuid = ? AND updated_at <= ?`,
		syncStatusSuccess, taskUUID, stepUUID, watermark); err != nil {
		applog.Warn("[CloudSync] 更新步骤会话同步成功状态失败", "task_uuid", taskUUID, "step_uuid", stepUUID, "error", err.Error())
	}
}

// markStepFailed records the failure reason on the step's pending session rows.
func (s *SessionSyncService) markStepFailed(ctx context.Context, taskUUID, stepUUID, reason string) {
	truncated := truncateRunes(reason, syncErrorMaxRunes)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE gt_cli_sessions
		 SET cloud_sync_status = ?, cloud_sync_error = ?
		 WHERE task_uuid = ? AND step_uuid = ?
		   AND (cloud_sync_status <> ? OR updated_at > cloud_synced_at)`,
		syncStatusFailed, truncated, taskUUID, stepUUID, syncStatusSuccess); err != nil {
		applog.Warn("[CloudSync] 更新步骤会话同步失败状态失败", "task_uuid", taskUUID, "step_uuid", stepUUID, "error", err.Error())
	}
}

// truncateRunes shortens a string to at most max runes.
func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
