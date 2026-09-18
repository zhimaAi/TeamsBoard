package localserver

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/i18n"
	"goteams-client/internal/project"
)

const maxIconUploadRequestSize = MaxIconFileSize + 1024*1024

type ProjectsHandler struct {
	db        func() *sql.DB
	iconStore *IconStore
}

func NewProjectsHandler(db func() *sql.DB, iconStore *IconStore) *ProjectsHandler {
	return &ProjectsHandler{db: db, iconStore: iconStore}
}

func (h *ProjectsHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.list)
	r.POST("", h.create)
	r.GET("/:uuid", h.get)
	r.PUT("/:uuid", h.update)
	r.DELETE("/:uuid", h.delete)
}

func (h *ProjectsHandler) service(c *gin.Context) (*project.Service, bool) {
	if h.db == nil || h.db() == nil {
		i18n.LocalServerError(c, http.StatusServiceUnavailable, errors.New("本地任务数据库尚未就绪"))
		return nil, false
	}
	return project.NewService(h.db()), true
}

func (h *ProjectsHandler) list(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	items, err := svc.List(c.Request.Context())
	if err != nil {
		i18n.LocalServerError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ProjectsHandler) get(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Get(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		projectError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *ProjectsHandler) create(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	input, newIconURL, err := h.bindInput(c)
	if err != nil {
		projectIconError(c, err)
		return
	}
	if newIconURL != "" {
		defer func() {
			if err != nil {
				_ = h.iconStore.Remove(newIconURL)
			}
		}()
	}
	item, err := svc.Create(c.Request.Context(), input)
	if err != nil {
		projectError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *ProjectsHandler) update(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	previous, err := svc.Get(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		projectError(c, err)
		return
	}
	input, newIconURL, err := h.bindInput(c)
	if err != nil {
		projectIconError(c, err)
		return
	}
	item, err := svc.Update(c.Request.Context(), c.Param("uuid"), input)
	if err != nil {
		if newIconURL != "" {
			_ = h.iconStore.Remove(newIconURL)
		}
		projectError(c, err)
		return
	}
	if previous.IconURL != item.IconURL {
		_ = h.iconStore.Remove(previous.IconURL)
	}
	c.JSON(http.StatusOK, item)
}

func (h *ProjectsHandler) delete(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Get(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		projectError(c, err)
		return
	}
	if err := svc.Delete(c.Request.Context(), c.Param("uuid")); err != nil {
		projectError(c, err)
		return
	}
	_ = h.iconStore.Remove(item.IconURL)
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *ProjectsHandler) bindInput(c *gin.Context) (project.Input, string, error) {
	var input project.Input
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		if err := c.ShouldBindJSON(&input); err != nil {
			return input, "", err
		}
		return input, "", nil
	}
	if h.iconStore == nil {
		return input, "", fmt.Errorf("本地图标存储尚未初始化")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxIconUploadRequestSize)
	input = project.Input{
		Name:     c.PostForm("name"),
		IconType: c.PostForm("icon_type"),
		IconURL:  c.PostForm("icon_url"),
		LocalDir: c.PostForm("local_dir"),
	}
	header, err := c.FormFile("icon_file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return input, "", nil
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) || strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			return input, "", ErrIconTooLarge
		}
		return input, "", fmt.Errorf("读取上传图片失败: %w", err)
	}
	iconURL, err := h.iconStore.Save(header)
	if err != nil {
		return input, "", err
	}
	input.IconType = "custom"
	input.IconURL = iconURL
	return input, iconURL, nil
}

func projectIconError(c *gin.Context, err error) {
	if errors.Is(err, ErrIconTooLarge) {
		i18n.LocalServerError(c, http.StatusRequestEntityTooLarge, err)
		return
	}
	i18n.LocalServerError(c, http.StatusBadRequest, err)
}

func projectError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, project.ErrNotFound):
		i18n.LocalServerError(c, http.StatusNotFound, err)
	case errors.Is(err, project.ErrInUse):
		i18n.LocalServerError(c, http.StatusConflict, err)
	default:
		i18n.LocalServerError(c, http.StatusBadRequest, err)
	}
}
