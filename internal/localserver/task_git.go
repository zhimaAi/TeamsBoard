package localserver

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"goteams-client/internal/i18n"
	"goteams-client/internal/taskgit"
)

func (h *TasksHandler) taskGitDirectory(ctx context.Context, c *gin.Context) (string, bool) {
	var directory string
	if h.taskDB() == nil {
		i18n.Error(c, http.StatusServiceUnavailable, "common_server_error", "")
		return "", false
	}
	// 任务通常把 CLI 工作目录保存在 work_dir；老任务可能只有 task_dir，
	// 此时回退到任务目录，保证分支读取和文档入口使用同一份可用路径。
	err := h.taskDB().QueryRowContext(ctx, `SELECT COALESCE(NULLIF(work_dir, ''), task_dir, '') FROM gt_tasks WHERE uuid = ?`, c.Param("uuid")).Scan(&directory)
	if errors.Is(err, sql.ErrNoRows) {
		i18n.Error(c, http.StatusNotFound, "localserver_task_not_found", "task_not_found")
		return "", false
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return "", false
	}
	if strings.TrimSpace(directory) == "" || !filepath.IsAbs(directory) {
		i18n.Error(c, http.StatusBadRequest, "taskgit_directory_unavailable", "directory_unavailable")
		return "", false
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		i18n.Error(c, http.StatusBadRequest, "taskgit_directory_unavailable", "directory_unavailable")
		return "", false
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		i18n.Error(c, http.StatusBadRequest, "taskgit_directory_unavailable", "directory_unavailable")
		return "", false
	}
	return directory, true
}

func canonicalTaskPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

// 当前任务或同一 Git 工作树有活动会话时禁止切换，避免影响其他任务。
// 当前任务会话即使尚未设置 work_dir，也始终受到保护。
func (h *TasksHandler) hasRunningTaskSession(ctx context.Context, taskUUID, directory string) (bool, error) {
	rows, err := h.taskDB().QueryContext(ctx, `
		SELECT s.task_uuid, COALESCE(NULLIF(s.work_dir, ''), NULLIF(t.work_dir, ''), t.task_dir, '')
		FROM gt_cli_sessions s
		LEFT JOIN gt_tasks t ON t.uuid = s.task_uuid
		WHERE s.status IN ('created', 'running', 'waiting_input', 'stop_requested')`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	targetRoot, targetErr := taskgit.WorktreeRoot(ctx, directory)
	if targetErr != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}
	for rows.Next() {
		var sessionTask, sessionDir string
		if err := rows.Scan(&sessionTask, &sessionDir); err != nil {
			return false, err
		}
		if sessionTask == taskUUID {
			return true, nil
		}
		if targetErr != nil || strings.TrimSpace(sessionDir) == "" {
			continue
		}
		sessionRoot, err := taskgit.WorktreeRoot(ctx, canonicalTaskPath(sessionDir))
		if err != nil && ctx.Err() != nil {
			return false, ctx.Err()
		}
		if err == nil {
			targetInfo, targetStatErr := os.Stat(targetRoot)
			sessionInfo, sessionStatErr := os.Stat(sessionRoot)
			// SameFile 同时处理路径别名和 Windows 盘符、目录大小写差异。
			if targetStatErr == nil && sessionStatErr == nil && os.SameFile(targetInfo, sessionInfo) {
				return true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func (h *TasksHandler) listTaskGitBranches(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	directory, ok := h.taskGitDirectory(ctx, c)
	if !ok {
		return
	}
	snapshot, err := taskgit.List(ctx, directory)
	if err != nil {
		i18n.Error(c, http.StatusBadRequest, "taskgit_read_failed", "git_read_failed")
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *TasksHandler) checkoutTaskGitBranch(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	var body struct {
		Branch string `json:"branch"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Branch == "" {
		i18n.Error(c, http.StatusBadRequest, "taskgit_branch_invalid", "branch_invalid")
		return
	}
	directory, ok := h.taskGitDirectory(ctx, c)
	if !ok {
		return
	}
	running, err := h.hasRunningTaskSession(ctx, c.Param("uuid"), directory)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	if running {
		i18n.Error(c, http.StatusConflict, "taskgit_task_running", "task_running")
		return
	}
	err = taskgit.Switch(ctx, directory, body.Branch)
	if err != nil {
		key, code := "taskgit_switch_failed", "git_switch_failed"
		switch {
		case errors.Is(err, taskgit.ErrInvalidBranch):
			key, code = "taskgit_branch_invalid", "branch_invalid"
		case errors.Is(err, taskgit.ErrNotRepository):
			key, code = "taskgit_not_repository", "not_repository"
		case errors.Is(err, taskgit.ErrSwitchMerging):
			key, code = "taskgit_switch_merging", "git_switch_merging"
		case errors.Is(err, taskgit.ErrSwitchRebasing):
			key, code = "taskgit_switch_rebasing", "git_switch_rebasing"
		case errors.Is(err, taskgit.ErrSwitchDirty):
			key, code = "taskgit_switch_dirty", "git_switch_dirty"
		case errors.Is(err, taskgit.ErrSwitchCheckedOut):
			key, code = "taskgit_switch_checked_out", "git_switch_checked_out"
		}
		i18n.Error(c, http.StatusBadRequest, key, code)
		return
	}
	c.JSON(http.StatusOK, gin.H{"branch": body.Branch})
}
