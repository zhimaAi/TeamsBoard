package localserver

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/project"
)

type ProjectsHandler struct{ db func() *sql.DB }

func NewProjectsHandler(db func() *sql.DB) *ProjectsHandler { return &ProjectsHandler{db: db} }

func (h *ProjectsHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.list)
	r.POST("", h.create)
	r.GET("/:uuid", h.get)
	r.PUT("/:uuid", h.update)
	r.DELETE("/:uuid", h.delete)
}

func (h *ProjectsHandler) service(c *gin.Context) (*project.Service, bool) {
	if h.db == nil || h.db() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "本地任务数据库尚未就绪"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	var input project.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Create(c.Request.Context(), input)
	if err != nil {
		projectError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *ProjectsHandler) update(c *gin.Context) {
	var input project.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Update(c.Request.Context(), c.Param("uuid"), input)
	if err != nil {
		projectError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *ProjectsHandler) delete(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.Delete(c.Request.Context(), c.Param("uuid")); err != nil {
		projectError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func projectError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, project.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, project.ErrInUse):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
