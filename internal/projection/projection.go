// Package projection provides task projection snapshot building, hashing, and sync-marking.
// It is shared by workflow (orchestrator live push) and cloud (WSClient offline backfill),
// eliminating the two nearly identical ~400-line snapshot-building code paths.
package projection

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"goteams-client/internal/protocol"
)

// BuildSnapshot builds a task projection from the local database
func BuildSnapshot(db *sql.DB, taskUUID string) (*protocol.TaskSnapshotData, error) {
	// Query the task main record (including work item / Agent / time / usage fields)
	var agentID, agentName, agentColor, cliType string
	var workflowVersion int
	var workItemType, workItemID, title string
	var workspaceID int64
	var status, execStatus string
	var taskStartedAt, taskFinishedAt int64
	var taskInputTokens, taskOutputTokens, taskTotalTokens int64
	err := db.QueryRow(
		`SELECT agent_id, agent_name_snapshot, agent_color_snapshot, cli_type, workflow_version,
		        work_item_type, work_item_id, workspace_id, title, status, execution_status,
		        started_at, finished_at, input_tokens, output_tokens, total_tokens
		 FROM gt_tasks WHERE uuid = ?`, taskUUID).
		Scan(&agentID, &agentName, &agentColor, &cliType, &workflowVersion,
			&workItemType, &workItemID, &workspaceID, &title, &status, &execStatus,
			&taskStartedAt, &taskFinishedAt, &taskInputTokens, &taskOutputTokens, &taskTotalTokens)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("任务不存在: %s", taskUUID)
	}
	if err != nil {
		return nil, fmt.Errorf("查询任务失败: %w", err)
	}

	// Query sync status
	var localRevision int
	var projectionHash string
	err = db.QueryRow(
		`SELECT local_revision, projection_hash FROM gt_task_sync_state WHERE task_uuid = ?`, taskUUID).
		Scan(&localRevision, &projectionHash)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("查询同步状态失败: %w", err)
	}

	// Query steps
	rows, err := db.Query(
		`SELECT step_key, name, step_order, status, execution_status, prompt_snapshot, output_summary, error_summary,
		        input_tokens, output_tokens, total_tokens, conversation_rounds, started_at, finished_at, duration_ms
		 FROM gt_task_steps WHERE task_uuid = ? ORDER BY step_order`,
		taskUUID)
	if err != nil {
		return nil, fmt.Errorf("查询步骤失败: %w", err)
	}
	defer rows.Close()

	var steps []protocol.StepSnapshot
	var currentStepKey, execSummary string
	var startedAt, finishedAt int64
	var usage protocol.UsageStats
	for rows.Next() {
		var s protocol.StepSnapshot
		var stepStatus, stepExecStatus string
		var started, finished, durationMs int64
		if err := rows.Scan(&s.StepKey, &s.StepName, &s.SortOrder, &stepStatus, &stepExecStatus,
			&s.PromptSummary, &s.OutputSummary, &s.ErrorSummary,
			&s.Usage.InputTokens, &s.Usage.OutputTokens, &s.Usage.TotalTokens, &s.Usage.ConversationRounds,
			&started, &finished, &durationMs); err != nil {
			continue
		}
		s.Status = stepExecStatus
		s.StartedAt = FormatMillisPtr(started)
		s.FinishedAt = FormatMillisPtr(finished)
		s.DurationMs = durationMs
		steps = append(steps, s)

		if stepExecStatus == protocol.ExecStatusRunning {
			currentStepKey = s.StepKey
			execSummary = s.OutputSummary
		} else if execSummary == "" {
			execSummary = s.OutputSummary
		}
		if started > 0 && (startedAt == 0 || started < startedAt) {
			startedAt = started
		}
		if finished > 0 && finished > finishedAt {
			finishedAt = finished
		}
		usage.InputTokens += s.Usage.InputTokens
		usage.OutputTokens += s.Usage.OutputTokens
		usage.TotalTokens += s.Usage.TotalTokens
	}

	// Close the step cursor and release the connection, ensuring later queries on gt_cli_sessions do not deadlock under MaxOpenConns(1)
	rows.Close()

	// Each step execution or continued chat adds a Session, so the Session count equals the actual conversation turns.
	var aiConversationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gt_cli_sessions WHERE task_uuid = ?`, taskUUID).Scan(&aiConversationCount); err != nil {
		aiConversationCount = 0
	}
	usage.ConversationRounds = aiConversationCount

	// Prefer task-level aggregates (if the aggregator maintains them), otherwise fall back to step aggregates
	if taskStartedAt > 0 {
		startedAt = taskStartedAt
	}
	if taskFinishedAt > 0 {
		finishedAt = taskFinishedAt
	}
	if taskTotalTokens > 0 || taskInputTokens > 0 || taskOutputTokens > 0 {
		usage.InputTokens = int(taskInputTokens)
		usage.OutputTokens = int(taskOutputTokens)
		usage.TotalTokens = int(taskTotalTokens)
	}

	sessions, err := buildSessionSnapshots(db, taskUUID)
	if err != nil {
		return nil, err
	}
	conversations := protocol.GroupSessionSnapshots(sessions)

	agentIDInt, _ := strconv.ParseInt(agentID, 10, 64)
	workItemIDInt, _ := strconv.ParseInt(workItemID, 10, 64)

	snapshot := &protocol.TaskSnapshotData{
		LocalTaskUUID:  taskUUID,
		LocalRevision:  localRevision,
		ProjectionHash: projectionHash,
		WorkItem: protocol.WorkItemSnapshot{
			Type:        workItemType,
			ID:          workItemIDInt,
			WorkspaceID: workspaceID,
			Title:       title,
		},
		Agent: protocol.AgentSnapshot{
			ID:              agentIDInt,
			Name:            agentName,
			Color:           agentColor,
			CLIType:         cliType,
			WorkflowVersion: workflowVersion,
		},
		TaskStatus:         status,
		ExecutionStatus:    execStatus,
		CurrentStepKey:     currentStepKey,
		ExecutionSummary:   execSummary,
		StartedAt:          FormatMillisPtr(startedAt),
		FinishedAt:         FormatMillisPtr(finishedAt),
		LastLocalUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Usage:              usage,
		Steps:              steps,
		Conversations:      conversations,
	}

	return snapshot, nil
}

// FormatMillisPtr formats a millisecond timestamp into an RFC3339 pointer (returns nil for zero value)
func FormatMillisPtr(ms int64) *string {
	if ms <= 0 {
		return nil
	}
	s := time.UnixMilli(ms).UTC().Format(time.RFC3339)
	return &s
}

// ComputeProjectionHash computes the projection hash
func ComputeProjectionHash(snapshot *protocol.TaskSnapshotData) string {
	// Normalize whitelisted fields to JSON then SHA-256
	// Only project key fields such as task status, execution status, and step status
	normalized := fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		snapshot.LocalTaskUUID,
		snapshot.TaskStatus,
		snapshot.ExecutionStatus,
		snapshot.CurrentStepKey,
		snapshot.ExecutionSummary,
		snapshot.LocalRevision,
	)

	for _, step := range snapshot.Steps {
		normalized += "|" + step.StepKey + ":" + step.Status + ":" + step.OutputSummary
	}
	for _, conversation := range snapshot.Conversations {
		normalized += "|" + conversation.ConversationUUID + ":" + conversation.StepKey + ":" +
			conversation.Status + ":" + strconv.Itoa(conversation.Content.RoundCount)
		for _, round := range conversation.Content.Rounds {
			normalized += ":" + strconv.Itoa(round.RoundNo) + ":" + round.UserMessage
			for _, message := range round.AssistantMessages {
				normalized += ":" + strconv.Itoa(message.Sequence) + ":" + message.Content
			}
		}
	}

	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// buildSessionSnapshots builds the cloud index for each CLI Session under a task.
func buildSessionSnapshots(db *sql.DB, taskUUID string) ([]protocol.SessionSnapshot, error) {
	rows, err := db.Query(
		`SELECT s.uuid, s.conversation_uuid, s.parent_session_uuid,
		        ts.step_key, s.run_no, s.status, s.cli_type,
		        substr(s.prompt_snapshot, 1, 8192),
		        COALESCE((
		            SELECT substr(trim(json_extract(e.payload_json, '$.content')), 1, 16384)
		            FROM gt_session_events e
		            WHERE e.session_uuid = s.uuid
		              AND e.event_type = 'message'
		              AND json_valid(e.payload_json)
		              AND trim(COALESCE(json_extract(e.payload_json, '$.content'), '')) <> ''
		            ORDER BY e.sequence DESC
		            LIMIT 1
		        ), (
		            SELECT cs.response_summary
		            FROM gt_conversation_summaries cs
		            WHERE cs.session_uuid = s.uuid
		            ORDER BY cs.created_at DESC
		            LIMIT 1
		        ), ''),
		        substr(s.error_message, 1, 8192),
		        s.input_tokens, s.output_tokens, s.total_tokens,
		        s.started_at, s.finished_at, s.duration_ms
		 FROM gt_cli_sessions s
		 JOIN gt_task_steps ts ON ts.uuid = s.step_uuid
		 WHERE s.task_uuid = ?
		 ORDER BY ts.step_order ASC, s.created_at ASC, s.rowid ASC`,
		taskUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询 Session 快照失败: %w", err)
	}
	defer rows.Close()

	sessions := make([]protocol.SessionSnapshot, 0)
	for rows.Next() {
		var session protocol.SessionSnapshot
		var startedAt, finishedAt int64
		if err := rows.Scan(
			&session.SessionUUID,
			&session.ConversationUUID,
			&session.ParentSessionUUID,
			&session.StepKey,
			&session.RunNo,
			&session.Status,
			&session.CLIType,
			&session.RequestSummary,
			&session.ResponseSummary,
			&session.ErrorSummary,
			&session.Usage.InputTokens,
			&session.Usage.OutputTokens,
			&session.Usage.TotalTokens,
			&startedAt,
			&finishedAt,
			&session.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("读取 Session 快照失败: %w", err)
		}
		session.Usage.ConversationRounds = 1
		session.StartedAt = FormatMillisPtr(startedAt)
		session.FinishedAt = FormatMillisPtr(finishedAt)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 Session 快照失败: %w", err)
	}
	rows.Close()

	if err := attachSessionMessages(db, taskUUID, sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func attachSessionMessages(db *sql.DB, taskUUID string, sessions []protocol.SessionSnapshot) error {
	if len(sessions) == 0 {
		return nil
	}

	sessionIndexes := make(map[string]int, len(sessions))
	for index := range sessions {
		sessions[index].Messages = make([]protocol.SessionMessageSnapshot, 0)
		sessionIndexes[sessions[index].SessionUUID] = index
	}

	rows, err := db.Query(
		`SELECT e.session_uuid, e.sequence,
		        substr(trim(json_extract(e.payload_json, '$.content')), 1, 16384),
		        e.created_at
		 FROM gt_session_events e
		 JOIN gt_cli_sessions s ON s.uuid = e.session_uuid
		 WHERE s.task_uuid = ?
		   AND e.event_type = 'message'
		   AND json_valid(e.payload_json)
		   AND trim(COALESCE(json_extract(e.payload_json, '$.content'), '')) <> ''
		 ORDER BY s.created_at ASC, s.rowid ASC, e.sequence ASC`,
		taskUUID,
	)
	if err != nil {
		return fmt.Errorf("查询 Session 消息快照失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sessionUUID, content string
		var sequence int
		var createdAt int64
		if err := rows.Scan(&sessionUUID, &sequence, &content, &createdAt); err != nil {
			return fmt.Errorf("读取 Session 消息快照失败: %w", err)
		}
		index, ok := sessionIndexes[sessionUUID]
		if !ok {
			continue
		}
		sessions[index].Messages = append(sessions[index].Messages, protocol.SessionMessageSnapshot{
			Sequence:  sequence,
			Role:      "assistant",
			Content:   content,
			Timestamp: time.UnixMilli(createdAt).UTC().Format(time.RFC3339Nano),
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 Session 消息快照失败: %w", err)
	}
	return nil
}

// MarkDirty marks a task as dirty (needs sync)
func MarkDirty(db *sql.DB, taskUUID string, revision int, hash, eventID string) error {
	now := time.Now().UnixMilli()
	_, err := db.Exec(
		`UPDATE gt_task_sync_state SET sync_dirty = 1, local_revision = ?, projection_hash = ?, pending_event_id = ?, updated_at = ? WHERE task_uuid = ?`,
		revision, hash, eventID, now, taskUUID)
	if err != nil {
		return fmt.Errorf("标记 dirty 失败: %w", err)
	}
	return nil
}

// Truncate safely truncates a UTF-8 string by bytes, replacing the overflow with "...".
func Truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	end := maxBytes
	for end > 0 && (s[end]&0xC0) == 0x80 {
		end--
	}
	return s[:end] + "..."
}
