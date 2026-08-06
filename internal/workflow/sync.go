package workflow

import (
	"github.com/google/uuid"

	"goteams-client/internal/applog"
)

// SyncTaskDelete notifies the cloud to sync-delete after a local task deletion.
// Push immediately when online; when offline, write to the gt_pending_task_deletes pending buffer,
// and have WSClient's periodic refresh / reconnect backfill it to the cloud, avoiding zombie tasks left in the cloud on offline delete.
func (o *Orchestrator) SyncTaskDelete(taskUUID string) {
	if taskUUID == "" {
		return
	}
	// Record pending delete (offline fallback)
	if _, err := o.db.Exec(
		`INSERT OR IGNORE INTO gt_pending_task_deletes (local_task_uuid, created_at) VALUES (?, ?)`,
		taskUUID, nowMillis()); err != nil {
		applog.Warn("[Orchestrator] syncDelete 记录待删失败", "task", taskUUID, "error", err)
	}
	if o.wsClient != nil && o.wsClient.IsConnected() {
		if err := o.wsClient.SendTaskDelete(taskUUID); err != nil {
			applog.Warn("[Orchestrator] syncDelete 推送失败", "task", taskUUID, "error", err)
			return
		}
		// Push succeeded, clear pending delete record
		if _, err := o.db.Exec(`DELETE FROM gt_pending_task_deletes WHERE local_task_uuid = ?`, taskUUID); err != nil {
			applog.Warn("[Orchestrator] syncDelete 清除待删失败", "task", taskUUID, "error", err)
		}
	}
}

// SyncTaskNow, after any task change (creation, status update, step completion, etc.),
// uniformly triggers one cloud sync: mark sync_dirty=1 and increment the local version number;
// if the WebSocket is connected, build the projection and push immediately; if not, only mark dirty,
// and let WSClient's fallback period or reconnect backfill the push.
func (o *Orchestrator) SyncTaskNow(taskUUID string) bool {
	if taskUUID == "" {
		return false
	}
	o.syncMu.Lock()
	defer o.syncMu.Unlock()

	snapshot, err := BuildSnapshot(o.db, taskUUID)
	if err != nil {
		applog.Warn("[Orchestrator] syncNow 构建快照失败", "task", taskUUID, "error", err)
		return false
	}

	revision := snapshot.LocalRevision + 1
	hash := ComputeProjectionHash(snapshot)
	eventID := uuid.New().String()
	if err := MarkDirty(o.db, taskUUID, revision, hash, eventID); err != nil {
		applog.Warn("[Orchestrator] syncNow 标记脏失败", "task", taskUUID, "error", err)
		return false
	}

	if o.wsClient != nil && o.wsClient.IsConnected() {
		snapshot.LocalRevision = revision
		snapshot.ProjectionHash = hash
		if err := o.wsClient.SendTaskSnapshot(snapshot); err != nil {
			applog.Warn("[Orchestrator] syncNow 推送失败", "task", taskUUID, "error", err)
		}
	}
	return true
}

// recomputeAndSyncSession recomputes step/task status and syncs to the cloud
func (o *Orchestrator) recomputeAndSyncSession(taskUUID, stepKey string) {
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
		return
	}
	o.SyncTaskNow(taskUUID)
}
