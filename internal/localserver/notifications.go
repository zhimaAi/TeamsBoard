package localserver

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type NotificationsHandler struct{ db func() *sql.DB }

func NewNotificationsHandler(db func() *sql.DB) *NotificationsHandler {
	return &NotificationsHandler{db: db}
}

func (h *NotificationsHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.list)
	r.PUT("/:uuid/read", h.read)
	r.PUT("/tasks/:task_uuid/read", h.readTask)
	r.PUT("/tasks/:task_uuid/archive", h.archiveTask)
}

func (h *NotificationsHandler) list(c *gin.Context) {
	rows, err := h.db().QueryContext(c.Request.Context(), `SELECT n.uuid, n.task_uuid, t.title, n.task_step_uuid,
		COALESCE(s.name, ''), n.step_order, n.progress_uuid, n.terminal_status, n.summary, n.is_read, n.created_at
		FROM gt_task_notifications n JOIN gt_tasks t ON t.uuid = n.task_uuid
		LEFT JOIN gt_task_steps s ON s.uuid = n.task_step_uuid
		WHERE n.is_archived = 0 ORDER BY n.created_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id, taskID, taskTitle, stepID, stepName, progressID, status, summary string
		var stepOrder int
		var isRead bool
		var createdAt int64
		if err := rows.Scan(&id, &taskID, &taskTitle, &stepID, &stepName, &stepOrder, &progressID, &status, &summary, &isRead, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, gin.H{"uuid": id, "task_uuid": taskID, "task_title": taskTitle,
			"task_step_uuid": stepID, "step_name": stepName, "step_sort_order": stepOrder,
			"progress_uuid": progressID, "status": status, "summary": summary, "is_read": isRead, "created_at": createdAt})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *NotificationsHandler) read(c *gin.Context) {
	now := time.Now().UnixMilli()
	result, err := h.db().ExecContext(c.Request.Context(), `UPDATE gt_task_notifications SET is_read = 1, read_at = ? WHERE uuid = ?`, now, c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "通知不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func markTaskNotificationsRead(ctx context.Context, db *sql.DB, taskUUID string, isRead bool) error {
	now := time.Now().UnixMilli()
	readAt := int64(0)
	if isRead {
		readAt = now
	}
	_, err := db.ExecContext(ctx, `UPDATE gt_task_notifications SET is_read = ?, read_at = ? WHERE task_uuid = ?`, isRead, readAt, taskUUID)
	return err
}

func (h *NotificationsHandler) readTask(c *gin.Context) {
	var body struct {
		IsRead bool `json:"is_read"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := markTaskNotificationsRead(c.Request.Context(), h.db(), c.Param("task_uuid"), body.IsRead); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "is_read": body.IsRead})
}

func (h *NotificationsHandler) archiveTask(c *gin.Context) {
	var body struct {
		IsArchived bool `json:"is_archived"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.db().ExecContext(c.Request.Context(), `UPDATE gt_task_notifications SET is_archived = ? WHERE task_uuid = ?`, body.IsArchived, c.Param("task_uuid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var exists int
		if err = h.db().QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM gt_tasks WHERE uuid = ?`, c.Param("task_uuid")).Scan(&exists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "is_archived": body.IsArchived})
}
