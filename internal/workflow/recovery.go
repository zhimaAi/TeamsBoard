package workflow

import (
	"fmt"

	"goteams-client/internal/applog"
)

// HandleCrashRecovery handles crash recovery
func (o *Orchestrator) HandleCrashRecovery() error {
	// Mark all active Sessions as interrupted
	_, err := o.db.Exec(
		`UPDATE gt_cli_sessions SET status = ?, finished_at = ?, updated_at = ? WHERE status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		SessionStatusInterrupted, nowMillis(), nowMillis())
	if err != nil {
		return fmt.Errorf("崩溃恢复失败: %w", err)
	}

	// First collect all affected task_uuid values, to avoid a connection-pool deadlock from opening a transaction while rows are still open
	rows, err := o.db.Query(`SELECT DISTINCT task_uuid FROM gt_cli_sessions WHERE status = ?`, SessionStatusInterrupted)
	if err != nil {
		return fmt.Errorf("崩溃恢复查询受影响任务失败: %w", err)
	}
	var taskUUIDs []string
	for rows.Next() {
		var taskUUID string
		if err := rows.Scan(&taskUUID); err != nil {
			rows.Close()
			return fmt.Errorf("崩溃恢复扫描任务 UUID 失败: %w", err)
		}
		taskUUIDs = append(taskUUIDs, taskUUID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("崩溃恢复遍历任务列表失败: %w", err)
	}
	rows.Close()

	// Re-aggregate the status of all affected tasks.
	// A single task failure does not block the overall recovery flow, but it must log, to avoid untraceable state corruption.
	for _, taskUUID := range taskUUIDs {
		tx, err := o.db.Begin()
		if err != nil {
			applog.Warn("崩溃恢复开启事务失败", "task_uuid", taskUUID, "error", err.Error())
			continue
		}
		stepRows, err := tx.Query(`SELECT step_key FROM gt_task_steps WHERE task_uuid = ?`, taskUUID)
		if err != nil {
			tx.Rollback()
			applog.Warn("崩溃恢复查询任务步骤失败", "task_uuid", taskUUID, "error", err.Error())
			continue
		}
		var stepKeys []string
		var scanErr error
		for stepRows.Next() {
			var sk string
			if scanErr = stepRows.Scan(&sk); scanErr != nil {
				break
			}
			stepKeys = append(stepKeys, sk)
		}
		if scanErr == nil {
			scanErr = stepRows.Err()
		}
		stepRows.Close()
		if scanErr != nil {
			tx.Rollback()
			applog.Warn("崩溃恢复读取任务步骤失败", "task_uuid", taskUUID, "error", scanErr.Error())
			continue
		}

		for _, sk := range stepKeys {
			if err := RecomputeStepStatus(tx, taskUUID, sk); err != nil {
				applog.Warn("崩溃恢复重算步骤状态失败", "task_uuid", taskUUID, "step_key", sk, "error", err.Error())
			}
		}
		if err := RecomputeTaskStatus(tx, taskUUID); err != nil {
			applog.Warn("崩溃恢复重算任务状态失败", "task_uuid", taskUUID, "error", err.Error())
		}
		if err := tx.Commit(); err != nil {
			applog.Warn("崩溃恢复提交事务失败", "task_uuid", taskUUID, "error", err.Error())
			continue
		}
		// Crash recovery caused a task state change: trigger one cloud sync
		o.SyncTaskNow(taskUUID)
	}

	return nil
}

// ReconcileExecutionStates fixes the derived states produced by the old "any past round failure means step failure" behavior.
// Only recompute steps inconsistent with the latest round of Session, and after commit generate new ones via the standard sync path:
// revision, projection hash, and event ID, ensuring the cloud eventually receives the corrected step/task status.
func (o *Orchestrator) ReconcileExecutionStates() error {
	rows, err := o.db.Query(
		`SELECT ts.task_uuid, ts.step_key, ts.execution_status,
		        (
		            SELECT s.status
		            FROM gt_cli_sessions s
		            WHERE s.step_uuid = ts.uuid
		            ORDER BY s.created_at DESC, s.rowid DESC
		            LIMIT 1
		        ) AS latest_session_status
		 FROM gt_task_steps ts
		 WHERE EXISTS (
		     SELECT 1 FROM gt_cli_sessions s WHERE s.step_uuid = ts.uuid
		 )`,
	)
	if err != nil {
		return fmt.Errorf("查询待修复执行状态失败: %w", err)
	}

	taskSteps := make(map[string]map[string]bool)
	for rows.Next() {
		var taskUUID, stepKey, currentStatus, latestSessionStatus string
		if err := rows.Scan(&taskUUID, &stepKey, &currentStatus, &latestSessionStatus); err != nil {
			rows.Close()
			return fmt.Errorf("读取待修复执行状态失败: %w", err)
		}
		expectedStatus := executionStatusForLatestSession(latestSessionStatus, true)
		if currentStatus != expectedStatus {
			if taskSteps[taskUUID] == nil {
				taskSteps[taskUUID] = make(map[string]bool)
			}
			taskSteps[taskUUID][stepKey] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("遍历待修复执行状态失败: %w", err)
	}
	rows.Close()

	// The data migration puts all existing execution records into a one-time repair queue. Even if the local state happens to be already correct,
	// it still re-runs SyncTaskNow to overwrite the possibly still-cached old failure state in the cloud.
	queueRows, err := o.db.Query(`SELECT task_uuid FROM gt_execution_state_repair_queue`)
	if err != nil {
		return fmt.Errorf("查询执行状态修复队列失败: %w", err)
	}
	var queuedTaskUUIDs []string
	for queueRows.Next() {
		var taskUUID string
		if err := queueRows.Scan(&taskUUID); err != nil {
			queueRows.Close()
			return fmt.Errorf("读取执行状态修复队列失败: %w", err)
		}
		queuedTaskUUIDs = append(queuedTaskUUIDs, taskUUID)
	}
	if err := queueRows.Err(); err != nil {
		queueRows.Close()
		return fmt.Errorf("遍历执行状态修复队列失败: %w", err)
	}
	queueRows.Close()

	for _, taskUUID := range queuedTaskUUIDs {
		if taskSteps[taskUUID] == nil {
			taskSteps[taskUUID] = make(map[string]bool)
		}
		stepRows, err := o.db.Query(
			`SELECT DISTINCT ts.step_key
			 FROM gt_task_steps ts
			 JOIN gt_cli_sessions s ON s.step_uuid = ts.uuid
			 WHERE ts.task_uuid = ?`,
			taskUUID,
		)
		if err != nil {
			return fmt.Errorf("查询修复任务步骤失败: %w", err)
		}
		for stepRows.Next() {
			var stepKey string
			if err := stepRows.Scan(&stepKey); err != nil {
				stepRows.Close()
				return fmt.Errorf("读取修复任务步骤失败: %w", err)
			}
			taskSteps[taskUUID][stepKey] = true
		}
		if err := stepRows.Err(); err != nil {
			stepRows.Close()
			return fmt.Errorf("遍历修复任务步骤失败: %w", err)
		}
		stepRows.Close()
	}

	queuedTasks := make(map[string]bool, len(queuedTaskUUIDs))
	for _, taskUUID := range queuedTaskUUIDs {
		queuedTasks[taskUUID] = true
	}

	for taskUUID, stepKeys := range taskSteps {
		tx, err := o.db.Begin()
		if err != nil {
			return fmt.Errorf("开启执行状态修复事务失败: %w", err)
		}
		for stepKey := range stepKeys {
			if err := RecomputeStepStatus(tx, taskUUID, stepKey); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := RecomputeTaskStatus(tx, taskUUID); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("提交执行状态修复事务失败: %w", err)
		}
		syncMarked := o.SyncTaskNow(taskUUID)
		if queuedTasks[taskUUID] && syncMarked {
			if _, err := o.db.Exec(
				`DELETE FROM gt_execution_state_repair_queue WHERE task_uuid = ?`,
				taskUUID,
			); err != nil {
				return fmt.Errorf("清除执行状态修复队列失败: %w", err)
			}
		}
	}

	applog.Info("[Orchestrator] 执行状态修复完成", "tasks", len(taskSteps))
	return nil
}
