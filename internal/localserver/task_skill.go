package localserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"goteams-client/internal/i18n"
	builtinskills "goteams-client/internal/skills"
	"goteams-client/internal/taskruntime"
)

const (
	maxTaskSkillBodyBytes = 512 * 1024
	maxActivityContent    = 64 * 1024
	maxActivityThreadID   = 256
)

type taskSkillHistoryItem struct {
	UUID      string `json:"uuid"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Tool      string `json:"tool"`
	ThreadID  string `json:"thread_id,omitempty"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// RegisterSkillRoutes exposes the task-scoped surface used by the bundled
// teamsboard Skill. The route group applies the dedicated loopback guard.
func (h *TasksHandler) RegisterSkillRoutes(r *gin.RouterGroup) {
	r.GET("/tasks/:uuid", h.getTaskForSkill)
	r.PUT("/tasks/:uuid/status", h.updateTaskStatusFromSkill)
	r.PUT("/tasks/:uuid/vibe-session", h.updateVibeCodingSessionFromSkill)
	r.POST("/tasks/:uuid/activities", h.createSkillActivity)
}

type vibeCodingSession struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
}

func (h *TasksHandler) getCodexCapability(c *gin.Context) {
	reasonCode := h.codexSkillReason
	response := gin.H{"available": h.codexSkillReady, "reason_code": reasonCode}
	if !h.codexSkillReady {
		if reasonCode == "" {
			reasonCode = builtinskills.CapabilityReasonInstallationFail
			response["reason_code"] = reasonCode
		}
		response["message"] = i18n.T(c, codexCapabilityMessageKey(reasonCode))
	}
	c.JSON(http.StatusOK, response)
}

func codexCapabilityMessageKey(reasonCode string) string {
	if reasonCode == builtinskills.CapabilityReasonNotManaged {
		return "localserver_codex_skill_not_managed"
	}
	return "localserver_codex_skill_install_failed"
}

func (h *TasksHandler) getTaskForSkill(c *gin.Context) {
	taskUUID, ok := validateTaskSkillUUID(c)
	if !ok {
		return
	}
	var sourceType, title, description, status, legacyWorkDir, executionMode, executionTool string
	var createdAt, updatedAt int64
	err := h.taskDB().QueryRowContext(c.Request.Context(), `SELECT source_type, title, content_snapshot, status, work_dir,
		execution_mode, execution_tool, created_at, updated_at FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(
		&sourceType, &title, &description, &status, &legacyWorkDir, &executionMode, &executionTool, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "localserver_task_not_found", "task_not_found")
		return
	}
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if !isCodexTask(executionMode, executionTool) {
		i18n.Error(c, http.StatusConflict, "localserver_task_not_codex", "task_not_codex")
		return
	}
	workDirs, err := h.loadTaskWorkDirs(taskUUID, legacyWorkDir)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	history, err := h.loadTaskSkillHistory(c.Request.Context(), taskUUID)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"task": gin.H{
			"uuid": taskUUID, "source_type": sourceType, "title": title, "description": description,
			"status": taskSkillPublicStatus(status), "execution_mode": executionMode, "execution_tool": executionTool,
			"work_dirs": workDirs, "created_at": createdAt, "updated_at": updatedAt,
		},
		"history": history, "allowed_statuses": []string{"pending", "in_progress", "blocked", "done"},
	})
}

func (h *TasksHandler) getCodexContext(c *gin.Context) {
	if !h.codexSkillReady {
		reasonCode := h.codexSkillReason
		if reasonCode == "" {
			reasonCode = builtinskills.CapabilityReasonInstallationFail
		}
		i18n.Error(c, http.StatusConflict, codexCapabilityMessageKey(reasonCode), reasonCode)
		return
	}
	taskUUID, ok := validateTaskSkillUUID(c)
	if !ok {
		return
	}
	var legacyWorkDir, executionMode, executionTool, sessionJSON string
	err := h.taskDB().QueryRowContext(c.Request.Context(), `SELECT work_dir, execution_mode, execution_tool,
		vibe_coding_session_json FROM gt_tasks WHERE uuid=?`, taskUUID).
		Scan(&legacyWorkDir, &executionMode, &executionTool, &sessionJSON)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "localserver_task_not_found", "task_not_found")
		return
	}
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if !isCodexTask(executionMode, executionTool) {
		i18n.Error(c, http.StatusConflict, "localserver_task_not_codex", "task_not_codex")
		return
	}
	workDirs, err := h.loadTaskWorkDirs(taskUUID, legacyWorkDir)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if len(workDirs) == 0 || strings.TrimSpace(workDirs[0]) == "" {
		i18n.Error(c, http.StatusConflict, "localserver_task_work_dir_required", "task_work_dir_required")
		return
	}
	skillPath := filepath.Join(h.skillsRoot, builtinskills.TaskSkillName, "SKILL.md")
	linkPath := encodeMarkdownLocalPath(skillPath)
	prompt := i18n.Format(c, "localserver_codex_task_prompt", i18n.Params{"SkillPath": linkPath, "TaskUUID": taskUUID})
	response := gin.H{"task_uuid": taskUUID, "work_dir": workDirs[0], "prompt": prompt}
	if session, ok := validVibeCodingSession(sessionJSON, executionTool); ok {
		response["thread_id"] = session.ThreadID
	}
	c.JSON(http.StatusOK, response)
}

func (h *TasksHandler) updateVibeCodingSessionFromSkill(c *gin.Context) {
	taskUUID, ok := validateTaskSkillUUID(c)
	if !ok {
		return
	}
	var body vibeCodingSession
	if !decodeTaskSkillJSON(c, &body) {
		return
	}
	body.Type = strings.ToLower(strings.TrimSpace(body.Type))
	body.ThreadID = strings.TrimSpace(body.ThreadID)
	if body.Type == "" {
		i18n.Error(c, http.StatusBadRequest, "localserver_vibe_session_type_invalid", "vibe_session_type_invalid")
		return
	}

	tx, err := h.taskDB().BeginTx(c.Request.Context(), nil)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	var executionMode, executionTool string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT execution_mode, execution_tool FROM gt_tasks WHERE uuid=?`, taskUUID).
		Scan(&executionMode, &executionTool); err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "localserver_task_not_found", "task_not_found")
		return
	} else if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if executionMode != taskruntime.ExecutionModeVibeCoding {
		i18n.Error(c, http.StatusConflict, "localserver_task_not_vibe_coding", "task_not_vibe_coding")
		return
	}
	if body.Type != strings.ToLower(strings.TrimSpace(executionTool)) {
		i18n.Error(c, http.StatusConflict, "localserver_vibe_session_type_mismatch", "vibe_session_type_mismatch")
		return
	}
	if body.Type != taskruntime.ExecutionToolCodex {
		i18n.Error(c, http.StatusBadRequest, "localserver_vibe_session_type_invalid", "vibe_session_type_invalid")
		return
	}
	normalizedThreadID, valid := normalizeCodexThreadID(body.ThreadID)
	if !valid {
		i18n.Error(c, http.StatusBadRequest, "localserver_vibe_session_thread_id_invalid", "vibe_session_thread_id_invalid")
		return
	}
	body.ThreadID = normalizedThreadID
	sessionJSON, err := json.Marshal(body)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(c.Request.Context(), `UPDATE gt_tasks SET vibe_coding_session_json=?, updated_at=?
		WHERE uuid=? AND execution_mode='vibe_coding' AND execution_tool=?`, string(sessionJSON), now, taskUUID, body.Type)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, rowsErr)
		return
	} else if rows != 1 {
		i18n.Error(c, http.StatusConflict, "localserver_vibe_session_type_mismatch", "vibe_session_type_mismatch")
		return
	}
	if err = tx.Commit(); err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	h.notifyTaskChanged(taskUUID)
	c.JSON(http.StatusOK, gin.H{
		"task_uuid": taskUUID, "type": body.Type, "thread_id": body.ThreadID, "updated_at": now,
	})
}

func validVibeCodingSession(raw, expectedType string) (vibeCodingSession, bool) {
	var session vibeCodingSession
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &session) != nil {
		return vibeCodingSession{}, false
	}
	session.Type = strings.ToLower(strings.TrimSpace(session.Type))
	if session.Type == "" || session.Type != strings.ToLower(strings.TrimSpace(expectedType)) {
		return vibeCodingSession{}, false
	}
	if session.Type != taskruntime.ExecutionToolCodex {
		return vibeCodingSession{}, false
	}
	threadID, ok := normalizeCodexThreadID(session.ThreadID)
	if !ok {
		return vibeCodingSession{}, false
	}
	session.ThreadID = threadID
	return session, true
}

func normalizeCodexThreadID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	id, err := uuid.Parse(value)
	if err != nil || id.String() != strings.ToLower(value) {
		return "", false
	}
	return id.String(), true
}

func encodeMarkdownLocalPath(path string) string {
	normalized := filepath.ToSlash(path)
	segments := strings.Split(normalized, "/")
	for index, segment := range segments {
		if index == 0 && len(segment) == 2 && isASCIIAlpha(segment[0]) && segment[1] == ':' {
			continue
		}
		segments[index] = encodeMarkdownPathSegment(segment)
	}
	return strings.Join(segments, "/")
}

func encodeMarkdownPathSegment(segment string) string {
	const hex = "0123456789ABCDEF"
	var encoded strings.Builder
	for _, value := range []byte(segment) {
		if isURLUnreserved(value) {
			encoded.WriteByte(value)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hex[value>>4])
		encoded.WriteByte(hex[value&0x0f])
	}
	return encoded.String()
}

func isURLUnreserved(value byte) bool {
	return isASCIIAlpha(value) || value >= '0' && value <= '9' || strings.ContainsRune("-._~", rune(value))
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func (h *TasksHandler) updateTaskStatusFromSkill(c *gin.Context) {
	taskUUID, ok := validateTaskSkillUUID(c)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if !decodeTaskSkillJSON(c, &body) {
		return
	}
	publicStatus := strings.TrimSpace(body.Status)
	storedStatus := map[string]string{"pending": "pending", "in_progress": "active", "blocked": "blocked", "done": "done"}[publicStatus]
	if storedStatus == "" {
		i18n.Error(c, http.StatusBadRequest, "localserver_skill_status_invalid", "skill_status_invalid")
		return
	}
	now := time.Now().UnixMilli()
	result, err := h.taskDB().ExecContext(c.Request.Context(), `UPDATE gt_tasks SET status=?, updated_at=?
		WHERE uuid=? AND execution_mode='vibe_coding' AND execution_tool='codex'`, storedStatus, now, taskUUID)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if rows == 0 {
		h.writeSkillTaskAccessError(c, taskUUID)
		return
	}
	if storedStatus == "active" {
		if err := h.ensureVibeCodingConversation(c.Request.Context(), taskUUID, i18n.T(c, "localserver_vibe_coding_assigned_activity"), false); err != nil {
			i18n.LocalServerError(c, http.StatusInternalServerError, err)
			return
		}
	}
	if err := h.syncVibeCodingConversationStatus(c.Request.Context(), taskUUID, storedStatus); err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if storedStatus == "done" {
		_ = markTaskNotificationsRead(c.Request.Context(), h.taskDB(), taskUUID, true)
	}
	h.notifyTaskChanged(taskUUID)
	h.pushTaskSync(taskUUID)
	c.JSON(http.StatusOK, gin.H{"task_uuid": taskUUID, "status": publicStatus, "updated_at": now})
}

func (h *TasksHandler) createSkillActivity(c *gin.Context) {
	taskUUID, ok := validateTaskSkillUUID(c)
	if !ok {
		return
	}
	var body struct {
		Type     string `json:"type"`
		Content  string `json:"content"`
		Tool     string `json:"tool"`
		ThreadID string `json:"thread_id"`
	}
	if !decodeTaskSkillJSON(c, &body) {
		return
	}
	body.Type = strings.TrimSpace(body.Type)
	if body.Type != "question" && body.Type != "result" {
		i18n.Error(c, http.StatusBadRequest, "localserver_skill_activity_type_invalid", "skill_activity_type_invalid")
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" || utf8.RuneCountInString(body.Content) > maxActivityContent {
		i18n.Error(c, http.StatusBadRequest, "localserver_skill_activity_content_invalid", "skill_activity_content_invalid")
		return
	}
	body.Tool = strings.ToLower(strings.TrimSpace(body.Tool))
	if body.Tool == "" {
		body.Tool = taskruntime.ExecutionToolCodex
	}
	if body.Tool != taskruntime.ExecutionToolCodex {
		i18n.Error(c, http.StatusBadRequest, "localserver_skill_tool_invalid", "skill_tool_invalid")
		return
	}
	body.ThreadID = strings.TrimSpace(body.ThreadID)
	if utf8.RuneCountInString(body.ThreadID) > maxActivityThreadID {
		i18n.Error(c, http.StatusBadRequest, "localserver_skill_thread_id_invalid", "skill_thread_id_invalid")
		return
	}
	now := time.Now().UnixMilli()
	progressUUID := uuid.NewString()
	recordType, prompt, result := "initial_run", "", body.Content
	if body.Type == "question" {
		recordType, prompt, result = "user_question", body.Content, ""
	}
	tx, err := h.taskDB().BeginTx(c.Request.Context(), nil)
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	var executionMode, executionTool string
	if err := tx.QueryRowContext(c.Request.Context(), `SELECT execution_mode, execution_tool FROM gt_tasks WHERE uuid=?`, taskUUID).
		Scan(&executionMode, &executionTool); err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "localserver_task_not_found", "task_not_found")
		return
	} else if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if !isCodexTask(executionMode, executionTool) {
		i18n.Error(c, http.StatusConflict, "localserver_task_not_codex", "task_not_codex")
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(), `INSERT INTO gt_task_progress
		(uuid, task_uuid, task_pipeline_snapshot_uuid, task_step_uuid, session_uuid, record_type, user_prompt,
		 cli_type, model_name, status, final_result, execution_mode, external_session_id, created_at, started_at, finished_at)
		VALUES (?, ?, '', '', '', ?, ?, 'codex', '', 'success', ?, 'vibe_coding', ?, ?, ?, ?)`,
		progressUUID, taskUUID, recordType, prompt, result, body.ThreadID, now, now, now); err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(), `UPDATE gt_tasks SET updated_at=? WHERE uuid=?`, now, taskUUID); err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(), `UPDATE gt_task_notifications SET progress_uuid=?,
		is_read=0, read_at=0, is_archived=0, created_at=? WHERE task_uuid=?`, progressUUID, now, taskUUID); err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	if err := tx.Commit(); err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	h.notifyTaskChanged(taskUUID)
	h.pushTaskSync(taskUUID)
	c.JSON(http.StatusCreated, gin.H{
		"uuid": progressUUID, "type": body.Type, "content": body.Content, "tool": body.Tool,
		"thread_id": body.ThreadID, "created_at": now,
	})
}

func (h *TasksHandler) loadTaskSkillHistory(ctx context.Context, taskUUID string) ([]taskSkillHistoryItem, error) {
	rows, err := h.taskDB().QueryContext(ctx, `SELECT uuid, record_type, user_prompt, final_result, cli_type,
		external_session_id, status, created_at, finished_at FROM gt_task_progress WHERE task_uuid=? ORDER BY created_at, rowid`, taskUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]taskSkillHistoryItem, 0)
	for rows.Next() {
		var id, recordType, prompt, result, tool, threadID, status string
		var createdAt, finishedAt int64
		if err := rows.Scan(&id, &recordType, &prompt, &result, &tool, &threadID, &status, &createdAt, &finishedAt); err != nil {
			return nil, err
		}
		if recordType == "user_question" && strings.TrimSpace(prompt) != "" {
			items = append(items, taskSkillHistoryItem{UUID: id, Type: "question", Content: prompt, Tool: tool, ThreadID: threadID, Status: status, CreatedAt: createdAt})
		}
		if recordType != "user_question" && strings.TrimSpace(result) != "" {
			if finishedAt == 0 {
				finishedAt = createdAt
			}
			items = append(items, taskSkillHistoryItem{UUID: id, Type: "result", Content: result, Tool: tool, ThreadID: threadID, Status: status, CreatedAt: finishedAt})
		}
	}
	return items, rows.Err()
}

func validateTaskSkillUUID(c *gin.Context) (string, bool) {
	taskUUID := strings.TrimSpace(c.Param("uuid"))
	if _, err := uuid.Parse(taskUUID); err != nil {
		i18n.Error(c, http.StatusBadRequest, "localserver_task_uuid_invalid", "task_uuid_invalid")
		return "", false
	}
	return taskUUID, true
}

func isCodexTask(mode, tool string) bool {
	return mode == taskruntime.ExecutionModeVibeCoding && tool == taskruntime.ExecutionToolCodex
}

func (h *TasksHandler) writeSkillTaskAccessError(c *gin.Context, taskUUID string) {
	var mode, tool string
	if err := h.taskDB().QueryRowContext(c.Request.Context(), `SELECT execution_mode, execution_tool FROM gt_tasks WHERE uuid=?`, taskUUID).
		Scan(&mode, &tool); err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "localserver_task_not_found", "task_not_found")
	} else if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
	} else {
		i18n.Error(c, http.StatusConflict, "localserver_task_not_codex", "task_not_codex")
	}
}

func decodeTaskSkillJSON(c *gin.Context, target interface{}) bool {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		i18n.Error(c, http.StatusUnsupportedMediaType, "localserver_json_content_type_required", "json_content_type_required")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTaskSkillBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		i18n.Error(c, http.StatusBadRequest, "localserver_json_invalid", "json_invalid")
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		i18n.Error(c, http.StatusBadRequest, "localserver_json_single_object_required", "json_single_object_required")
		return false
	}
	return true
}

func taskSkillPublicStatus(status string) string {
	if status == "active" {
		return "in_progress"
	}
	return status
}

// ensureVibeCodingConversation makes an active Vibe Coding task discoverable
// from the conversation page. The assignment activity is only created when the
// task has no existing activity; the notification is an idempotent list anchor
// and is not rendered as an extra message. Conversation-page creation marks
// the anchor read until the first Skill activity arrives; other entry points
// create an unread anchor immediately.
func (h *TasksHandler) ensureVibeCodingConversation(ctx context.Context, taskUUID, activityContent string, initiallyRead bool) error {
	tx, err := h.taskDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var title, status, executionMode, executionTool string
	if err = tx.QueryRowContext(ctx, `SELECT title, status, execution_mode, execution_tool FROM gt_tasks WHERE uuid=?`, taskUUID).
		Scan(&title, &status, &executionMode, &executionTool); err != nil {
		return err
	}
	if status != "active" || executionMode != taskruntime.ExecutionModeVibeCoding {
		return tx.Commit()
	}

	now := time.Now().UnixMilli()
	progressUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("vibe-coding-assignment:"+taskUUID)).String()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO gt_task_progress
		(uuid, task_uuid, task_pipeline_snapshot_uuid, task_step_uuid, session_uuid, record_type, user_prompt,
		 cli_type, model_name, status, final_result, execution_mode, external_session_id, created_at, started_at, finished_at)
		SELECT ?, ?, '', '', '', 'initial_run', '', ?, '', 'success', ?, 'vibe_coding', '', ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM gt_task_progress WHERE task_uuid=?)`,
		progressUUID, taskUUID, executionTool, activityContent, now, now, now, taskUUID)
	if err != nil {
		return err
	}
	if inserted, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if inserted == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT uuid FROM gt_task_progress WHERE task_uuid=? ORDER BY created_at DESC, rowid DESC LIMIT 1`, taskUUID).
			Scan(&progressUUID); err != nil {
			return err
		}
	}

	var notificationUUID string
	err = tx.QueryRowContext(ctx, `SELECT uuid FROM gt_task_notifications WHERE task_uuid=? ORDER BY created_at DESC, rowid DESC LIMIT 1`, taskUUID).
		Scan(&notificationUUID)
	if err == sql.ErrNoRows {
		notificationUUID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("vibe-coding-notification:"+taskUUID)).String()
		notificationSessionUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("vibe-coding-notification-session:"+taskUUID)).String()
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO gt_task_notifications
			(uuid, task_uuid, task_step_uuid, step_order, progress_uuid, session_uuid,
			 terminal_status, title, summary, is_read, is_archived, created_at)
			VALUES (?, ?, '', 0, ?, ?, 'running', ?, ?, ?, 0, ?)`,
			notificationUUID, taskUUID, progressUUID, notificationSessionUUID, title, activityContent, initiallyRead, now); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if _, err = tx.ExecContext(ctx, `UPDATE gt_task_notifications SET progress_uuid=?, terminal_status='running',
		title=?, summary=?, is_archived=0 WHERE uuid=?`, progressUUID, title, activityContent, notificationUUID); err != nil {
		return err
	}

	return tx.Commit()
}

func (h *TasksHandler) syncVibeCodingConversationStatus(ctx context.Context, taskUUID, taskStatus string) error {
	notificationStatus := map[string]string{
		"pending": "created",
		"active":  "running",
		"blocked": "failed",
		"done":    "success",
	}[taskStatus]
	if notificationStatus == "" {
		return nil
	}
	_, err := h.taskDB().ExecContext(ctx, `UPDATE gt_task_notifications SET terminal_status=?, created_at=?
		WHERE task_uuid=? AND EXISTS (SELECT 1 FROM gt_tasks t WHERE t.uuid=? AND t.execution_mode='vibe_coding')`,
		notificationStatus, time.Now().UnixMilli(), taskUUID, taskUUID)
	return err
}
