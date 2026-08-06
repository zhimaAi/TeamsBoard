package workflow

import (
	"database/sql"
	"fmt"
	"strings"

	"goteams-client/internal/protocol"
)

// RecomputeStepStatus computes the execution status from the step's latest round of Sessions.
// Historical Sessions still count toward token, duration, and round statistics, but an early failure must not permanently override a later success.
func RecomputeStepStatus(tx *sql.Tx, taskUUID, stepKey string) error {
	// Query all Sessions under the step in creation order; the last one is the latest round that decides the step status.
	// rowid provides a stable order for records created within the same millisecond.
	rows, err := tx.Query(
		`SELECT status, input_tokens, output_tokens, total_tokens, started_at, finished_at, duration_ms
		 FROM gt_cli_sessions WHERE task_uuid = ? AND step_uuid = (
			SELECT uuid FROM gt_task_steps WHERE task_uuid = ? AND step_key = ?
		 )
		 ORDER BY created_at ASC, rowid ASC`,
		taskUUID, taskUUID, stepKey)
	if err != nil {
		return fmt.Errorf("查询 Session 状态失败: %w", err)
	}
	defer rows.Close()

	latestStatus := ""
	sessionCount := 0
	inputTokens := 0
	outputTokens := 0
	totalTokens := 0
	var startedAt, finishedAt, durationMs int64

	for rows.Next() {
		var status string
		var sessionInput, sessionOutput, sessionTotal int
		var sessionStarted, sessionFinished, sessionDuration int64
		if err := rows.Scan(
			&status,
			&sessionInput,
			&sessionOutput,
			&sessionTotal,
			&sessionStarted,
			&sessionFinished,
			&sessionDuration,
		); err != nil {
			continue
		}
		sessionCount++
		latestStatus = status
		inputTokens += sessionInput
		outputTokens += sessionOutput
		totalTokens += sessionTotal
		durationMs += sessionDuration
		if sessionStarted > 0 && (startedAt == 0 || sessionStarted < startedAt) {
			startedAt = sessionStarted
		}
		if sessionFinished > finishedAt {
			finishedAt = sessionFinished
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 Session 状态失败: %w", err)
	}
	rows.Close()

	var latestErrorSummary string
	_ = tx.QueryRow(
		`SELECT error_message
		 FROM gt_cli_sessions
		 WHERE task_uuid = ? AND step_uuid = (
		     SELECT uuid FROM gt_task_steps WHERE task_uuid = ? AND step_key = ?
		 )
		 ORDER BY created_at DESC, rowid DESC
		 LIMIT 1`,
		taskUUID, taskUUID, stepKey,
	).Scan(&latestErrorSummary)
	latestErrorSummary = strings.TrimSpace(latestErrorSummary)

	var outputSummary string
	_ = tx.QueryRow(
		`SELECT response_summary
		 FROM gt_conversation_summaries
		 WHERE step_key = ?
		   AND session_uuid IN (SELECT uuid FROM gt_cli_sessions WHERE task_uuid = ?)
		 ORDER BY created_at DESC LIMIT 1`,
		stepKey, taskUUID,
	).Scan(&outputSummary)

	execStatus := executionStatusForLatestSession(latestStatus, sessionCount > 0)

	now := nowMillis()
	_, err = tx.Exec(
		`UPDATE gt_task_steps
		 SET execution_status = ?,
		     output_summary = CASE WHEN ? <> '' THEN ? ELSE output_summary END,
		     error_summary = CASE
		         WHEN ? = 'success' THEN ''
		         WHEN ? <> '' THEN ?
		         ELSE error_summary
		     END,
		     input_tokens = ?, output_tokens = ?, total_tokens = ?,
		     conversation_rounds = ?, started_at = ?, finished_at = ?, duration_ms = ?,
		     updated_at = ?
		 WHERE task_uuid = ? AND step_key = ?`,
		execStatus, outputSummary, outputSummary, execStatus, latestErrorSummary, latestErrorSummary,
		inputTokens, outputTokens, totalTokens, sessionCount, startedAt, finishedAt, durationMs,
		now, taskUUID, stepKey)
	if err != nil {
		return fmt.Errorf("更新步骤执行状态失败: %w", err)
	}

	return nil
}

func executionStatusForLatestSession(sessionStatus string, hasSession bool) string {
	if !hasSession {
		return protocol.ExecStatusIdle
	}
	switch sessionStatus {
	case SessionStatusCreated, SessionStatusRunning, SessionStatusWaiting, SessionStatusStopRequested:
		return protocol.ExecStatusRunning
	case SessionStatusSuccess:
		return protocol.ExecStatusSuccess
	case SessionStatusStopped:
		return protocol.ExecStatusStopped
	case SessionStatusFailed, SessionStatusInterrupted:
		return protocol.ExecStatusFailed
	default:
		return protocol.ExecStatusFailed
	}
}

// RecomputeTaskStatus aggregates the Task execution status from all Step statuses
func RecomputeTaskStatus(tx *sql.Tx, taskUUID string) error {
	rows, err := tx.Query(
		`SELECT execution_status, input_tokens, output_tokens, total_tokens,
		        conversation_rounds, started_at, finished_at
		 FROM gt_task_steps WHERE task_uuid = ?`,
		taskUUID)
	if err != nil {
		return fmt.Errorf("查询步骤聚合数据失败: %w", err)
	}
	defer rows.Close()

	hasRunning := false
	hasFailed := false
	hasStopped := false
	allSuccess := true
	stepCount := 0
	inputTokens := 0
	outputTokens := 0
	totalTokens := 0
	conversationRounds := 0
	var startedAt, finishedAt int64

	for rows.Next() {
		var execStatus string
		var stepInput, stepOutput, stepTotal, stepRounds int
		var stepStarted, stepFinished int64
		if err := rows.Scan(
			&execStatus,
			&stepInput,
			&stepOutput,
			&stepTotal,
			&stepRounds,
			&stepStarted,
			&stepFinished,
		); err != nil {
			continue
		}
		stepCount++
		inputTokens += stepInput
		outputTokens += stepOutput
		totalTokens += stepTotal
		conversationRounds += stepRounds
		if stepStarted > 0 && (startedAt == 0 || stepStarted < startedAt) {
			startedAt = stepStarted
		}
		if stepFinished > finishedAt {
			finishedAt = stepFinished
		}
		switch execStatus {
		case protocol.ExecStatusRunning:
			hasRunning = true
			allSuccess = false
		case protocol.ExecStatusFailed:
			hasFailed = true
			allSuccess = false
		case protocol.ExecStatusStopped:
			hasStopped = true
			allSuccess = false
		case protocol.ExecStatusIdle:
			allSuccess = false
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历步骤聚合数据失败: %w", err)
	}
	rows.Close()

	execStatus := protocol.ExecStatusIdle
	if stepCount == 0 {
		execStatus = protocol.ExecStatusIdle
	} else if hasRunning {
		execStatus = protocol.ExecStatusRunning
	} else if hasFailed {
		execStatus = protocol.ExecStatusFailed
	} else if hasStopped {
		execStatus = protocol.ExecStatusStopped
	} else if allSuccess {
		execStatus = protocol.ExecStatusSuccess
	} else {
		execStatus = protocol.ExecStatusIdle
	}

	now := nowMillis()
	_, err = tx.Exec(
		`UPDATE gt_tasks
		 SET execution_status = ?,
		     input_tokens = ?, output_tokens = ?, total_tokens = ?,
		     conversation_rounds = ?, started_at = ?, finished_at = ?, updated_at = ?
		 WHERE uuid = ?`,
		execStatus, inputTokens, outputTokens, totalTokens, conversationRounds,
		startedAt, finishedAt, now, taskUUID)
	if err != nil {
		return fmt.Errorf("更新任务执行状态失败: %w", err)
	}

	return nil
}
