package configcenter

import (
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/applog"
	"goteams-client/internal/i18n"
	"goteams-client/internal/workspace"
)

// 工作空间（数据根）配置：
//   - GET /api/local/config/workspace 返回当前数据根、默认数据根、是否桌面端等；
//   - PUT /api/local/config/workspace 校验并执行搬迁，成功后要求重启客户端。
//
// 仅桌面端（Electron 宿主）允许修改；浏览器模式下该接口只读，
// 防止远程浏览器会话在用户无感知的情况下搬动本机数据目录。

// RegisterWorkspaceRoutes 注册工作空间配置路由（无需登录；与 cloud 配置同级）。
func (h *Handler) RegisterWorkspaceRoutes(r *gin.RouterGroup) {
	r.GET("/workspace", h.getWorkspace)
	r.PUT("/workspace", h.setWorkspace)
}

// workspaceResponse 是 GET 的响应结构，前端据此渲染工作空间区块。
type workspaceResponse struct {
	CurrentRoot string `json:"current_root"`
	DefaultRoot string `json:"default_root"`
	Relocated   bool   `json:"relocated"`
	IsDesktop   bool   `json:"is_desktop"`
	OS          string `json:"os"`
	DirName     string `json:"dir_name"` // 数据根目录名（.goteams），前端展示用
	RestartHint string `json:"restart_hint"`
}

func (h *Handler) getWorkspace(c *gin.Context) {
	root, err := workspace.ResolveDataRoot()
	if err != nil {
		// 解析失败（例如盘符丢失）也要把错误透出，前端可提示用户检查磁盘。
		c.JSON(http.StatusOK, gin.H{
			"current_root": "",
			"default_root": "",
			"relocated":    false,
			"is_desktop":   workspace.IsDesktopRuntime(),
			"os":           runtime.GOOS,
			"dir_name":     ".goteams",
			"error":        i18n.T(c, "configcenter_workspace_resolve_failed"),
		})
		return
	}
	defaultRoot := workspace.DefaultDataRoot()
	c.JSON(http.StatusOK, workspaceResponse{
		CurrentRoot: root,
		DefaultRoot: defaultRoot,
		Relocated:   !samePathFold(root, defaultRoot),
		IsDesktop:   workspace.IsDesktopRuntime(),
		OS:          runtime.GOOS,
		DirName:     ".goteams",
	})
}

type setWorkspaceRequest struct {
	// ParentDir 用户选择的目标父目录；数据根将搬迁到 <ParentDir>/.goteams。
	ParentDir string `json:"parent_dir"`
	// Confirm 必须为 true，前端确认弹窗收集用户明确同意后才置位。
	Confirm bool `json:"confirm"`
}

func (h *Handler) setWorkspace(c *gin.Context) {
	if !workspace.IsDesktopRuntime() {
		c.JSON(http.StatusForbidden, gin.H{"error": i18n.T(c, "configcenter_workspace_desktop_only")})
		return
	}
	// 搬迁成功后本次进程内所有服务仍持有旧数据根的句柄（数据库、日志、CLI
	// 会话目录），重启前再次搬迁会让指针与运行态彻底脫节，必须拒绝。
	if h.workspaceMoved {
		c.JSON(http.StatusConflict, gin.H{"error": i18n.T(c, "configcenter_workspace_restart_first")})
		return
	}

	var body setWorkspaceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_request_invalid")})
		return
	}
	body.ParentDir = strings.TrimSpace(body.ParentDir)
	if body.ParentDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_dir_required")})
		return
	}
	if !body.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_confirm_required")})
		return
	}

	plan, err := workspace.PlanWorkspace(body.ParentDir)
	if err != nil {
		writeWorkspacePlanError(c, err)
		return
	}

	newRoot, err := workspace.ApplyWorkspace(plan)
	if err != nil {
		applog.Error("工作空间搬迁失败",
			"from", plan.CurrentRoot,
			"to", plan.TargetRoot,
			"error", err.Error(),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "configcenter_workspace_move_failed")})
		return
	}

	applog.Info("工作空间已搬迁，等待用户重启客户端",
		"from", plan.CurrentRoot,
		"to", newRoot,
		"cross_volume", plan.CrossVolume,
	)
	h.workspaceMoved = true
	c.JSON(http.StatusOK, gin.H{
		"status":           "ok",
		"current_root":     newRoot,
		"previous_root":    plan.CurrentRoot,
		"cross_volume":     plan.CrossVolume,
		"restart_required": true,
	})
}

// writeWorkspacePlanError 把搬迁校验错误映射为稳定的用户可读提示。
func writeWorkspacePlanError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace.ErrWorkspaceConflict):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_target_has_data")})
		return
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "不存在"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_dir_not_exist")})
	case strings.Contains(msg, "不是目录"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_dir_not_dir")})
	case strings.Contains(msg, "不为空"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_dir_not_empty")})
	case strings.Contains(msg, "相同"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_same_dir")})
	case strings.Contains(msg, "当前工作空间之内"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_inside_current")})
	case strings.Contains(msg, "磁盘根目录"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_root_forbidden")})
	case strings.Contains(msg, "系统目录"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_system_forbidden")})
	case strings.Contains(msg, "不可写"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_not_writable")})
	case strings.Contains(msg, "被写入了内容"):
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_dir_changed")})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "configcenter_workspace_invalid")})
	}
}

// samePathFold 按平台特性比较两个路径是否指向同一位置。
func samePathFold(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
