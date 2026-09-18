package workflow

import (
	"context"
	"fmt"
)

// HandleCrashRecovery replays persisted expert routes and turns every non-terminal
// session left by the previous process into a normal interrupted terminal result.
// Going through finishSession also creates the required progress result and notification.
func (o *Orchestrator) HandleCrashRecovery() error {
	pendingRoutes, err := o.loadPendingExpertRoutes()
	if err != nil {
		return err
	}
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
	for _, value := range pendingRoutes {
		var childCount int
		if err := o.db.QueryRow(`SELECT COUNT(*) FROM gt_cli_sessions WHERE trigger_session_uuid=?`, value.sessionUUID).Scan(&childCount); err != nil {
			return fmt.Errorf("崩溃恢复检查专家团交接失败: %w", err)
		}
		if childCount > 0 {
			dispatchStatus, messageCode := "returned", expertDispatchCodeReturned
			if value.role == "leader" {
				dispatchStatus, messageCode = "dispatched", expertDispatchCodeDispatched
			}
			o.recordExpertDispatch(context.Background(), value.sessionUUID, value.taskUUID, dispatchStatus, messageCode)
			continue
		}
		o.routeExpertCompletion(value.sessionUUID, value.taskUUID, value.status, value.finalResult)
	}
	return nil
}

type pendingExpertRoute struct {
	sessionUUID string
	taskUUID    string
	status      string
	finalResult string
	role        string
}

func (o *Orchestrator) loadPendingExpertRoutes() ([]pendingExpertRoute, error) {
	rows, err := o.db.Query(`SELECT s.uuid, s.task_uuid, s.status, COALESCE(s.final_result,''), step.member_role
		FROM gt_cli_sessions s
		JOIN gt_task_steps step ON step.uuid=s.step_uuid
		JOIN gt_task_progress progress ON progress.session_uuid=s.uuid
		WHERE s.execution_mode='expert_group' AND progress.record_type<>'user_question'
		  AND progress.dispatch_status='routing_pending'
		ORDER BY s.created_at, s.rowid`)
	if err != nil {
		return nil, fmt.Errorf("崩溃恢复查询专家团路由失败: %w", err)
	}
	defer rows.Close()
	items := make([]pendingExpertRoute, 0)
	for rows.Next() {
		var value pendingExpertRoute
		if err := rows.Scan(&value.sessionUUID, &value.taskUUID, &value.status, &value.finalResult, &value.role); err != nil {
			return nil, fmt.Errorf("崩溃恢复读取专家团路由失败: %w", err)
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("崩溃恢复遍历专家团路由失败: %w", err)
	}
	return items, nil
}
