package workflow

import (
	"fmt"
)

// HandleCrashRecovery turns every non-terminal session left by the previous
// process into a normal interrupted terminal result. Going through
// finishSession also creates the required progress result and notification.
func (o *Orchestrator) HandleCrashRecovery() error {
	rows, err := o.db.Query(`SELECT s.uuid, s.task_uuid, ts.step_key
		FROM gt_cli_sessions s JOIN gt_task_steps ts ON ts.uuid = s.step_uuid
		WHERE s.status IN ('created', 'running', 'waiting_input', 'stop_requested')`)
	if err != nil {
		return fmt.Errorf("崩溃恢复查询会话失败: %w", err)
	}
	type item struct{ sessionUUID, taskUUID, stepKey string }
	items := make([]item, 0)
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.sessionUUID, &value.taskUUID, &value.stepKey); err != nil {
			rows.Close()
			return fmt.Errorf("崩溃恢复读取会话失败: %w", err)
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("崩溃恢复遍历会话失败: %w", err)
	}
	rows.Close()
	for _, value := range items {
		_, _ = o.db.Exec(`UPDATE gt_cli_sessions SET error_message = ? WHERE uuid = ?`, "客户端异常退出，CLI 会话已中断", value.sessionUUID)
		o.finishSession(value.sessionUUID, value.taskUUID, value.stepKey, SessionStatusInterrupted, "", 0, 0, "")
	}
	return nil
}
