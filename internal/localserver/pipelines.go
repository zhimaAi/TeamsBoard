package localserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/pipeline"
)

type PipelinesHandler struct {
	db          func() *sql.DB
	cloudClient func() *cloud.Client
}

func NewPipelinesHandler(db func() *sql.DB, cloudClients ...func() *cloud.Client) *PipelinesHandler {
	h := &PipelinesHandler{db: db}
	if len(cloudClients) > 0 {
		h.cloudClient = cloudClients[0]
	}
	return h
}

func (h *PipelinesHandler) RegisterCloudRoutes(r *gin.RouterGroup) {
	r.POST("/pipelines/sync", h.syncCloud)
	r.POST("/pipelines/sync-cloud", h.syncCloud)
}

func (h *PipelinesHandler) syncCloud(c *gin.Context) {
	if h.cloudClient == nil || h.cloudClient() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "云端客户端未初始化"})
		return
	}
	client := h.cloudClient()
	svc, ok := h.service(c)
	if !ok {
		return
	}
	result, err := syncCloudPipelines(c.Request.Context(), svc, client)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "synced": len(result)})
}

func syncCloudPipelines(ctx context.Context, svc *pipeline.Service, client *cloud.Client) ([]pipeline.Pipeline, error) {
	pipelines, err := client.GetPipelines(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]pipeline.CloudPipeline, 0, len(pipelines))
	for _, pl := range pipelines {
		snapshot, err := client.GetPipelineSnapshot(ctx, pl.ID)
		if err != nil {
			return nil, fmt.Errorf("同步云端流水线 %d 失败: %w", pl.ID, err)
		}
		item := pipeline.CloudPipeline{CloudPipelineID: strconv.FormatInt(snapshot.ID, 10), Name: snapshot.Name, Avatar: snapshot.Icon}
		for _, step := range snapshot.Steps {
			item.Steps = append(item.Steps, pipeline.CloudStep{CloudStepID: step.StepKey, SortOrder: step.SortOrder, Name: step.Name, Prompt: step.Prompt})
		}
		items = append(items, item)
	}
	return svc.SyncCloud(ctx, pipeline.CloudSourceKey(client.BaseURL()), items)
}

func (s *Server) triggerCloudPipelineSync() {
	syncDB, syncClient := s.currentSessionDB(), s.currentCloudClient()
	if syncDB == nil || syncClient == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := syncCloudPipelines(ctx, pipeline.NewService(syncDB), syncClient); err != nil {
			applog.Warn("[LocalServer] 登录后同步云端流水线失败", "error", err)
		}
	}()
}

func (h *PipelinesHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.list)
	r.POST("", h.create)
	r.GET("/:uuid", h.get)
	r.PUT("/:uuid", h.update)
	r.DELETE("/:uuid", h.delete)
	r.POST("/:uuid/steps", h.addStep)
	r.PUT("/:uuid/steps/reorder", h.reorderSteps)
	r.PUT("/:uuid/steps/:step_uuid", h.updateStep)
	r.DELETE("/:uuid/steps/:step_uuid", h.deleteStep)
}

func (h *PipelinesHandler) service(c *gin.Context) (*pipeline.Service, bool) {
	if h.db == nil || h.db() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "本地任务数据库尚未就绪"})
		return nil, false
	}
	return pipeline.NewService(h.db()), true
}

func (h *PipelinesHandler) list(c *gin.Context) {
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

func (h *PipelinesHandler) get(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Get(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *PipelinesHandler) create(c *gin.Context) {
	var input pipeline.PipelineInput
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *PipelinesHandler) update(c *gin.Context) {
	var input pipeline.PipelineInput
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
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *PipelinesHandler) delete(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.Delete(c.Request.Context(), c.Param("uuid")); err != nil {
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *PipelinesHandler) addStep(c *gin.Context) {
	var input pipeline.StepInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.AddStep(c.Request.Context(), c.Param("uuid"), input)
	if err != nil {
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *PipelinesHandler) updateStep(c *gin.Context) {
	var input pipeline.StepInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.UpdateStep(c.Request.Context(), c.Param("uuid"), c.Param("step_uuid"), input)
	if err != nil {
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *PipelinesHandler) deleteStep(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.DeleteStep(c.Request.Context(), c.Param("uuid"), c.Param("step_uuid")); err != nil {
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *PipelinesHandler) reorderSteps(c *gin.Context) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.ReorderSteps(c.Request.Context(), c.Param("uuid"), body.IDs); err != nil {
		pipelineError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func pipelineError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pipeline.ErrNotFound), errors.Is(err, pipeline.ErrStepNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
