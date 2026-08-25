package localserver

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"goteams-client/internal/applog"
	"goteams-client/internal/cloud"
	"goteams-client/internal/cloudsync"
	"goteams-client/internal/executor"
	"goteams-client/internal/pipeline"
	"goteams-client/internal/protocol"
	"goteams-client/internal/taskruntime"
	"goteams-client/internal/workflow"
)

// TasksHandler task-related HTTP handler
//
// Task data is always located in the application-scoped goteams.db.
type TasksHandler struct {
	sessionDB    func() *sql.DB
	orchestrator func() *workflow.Orchestrator
	cloudClient  func() *cloud.Client
	wsHub        *WSHub
	taskRoot     string
}

// NewTasksHandler creates task handler
func NewTasksHandler(sessionDB func() *sql.DB, orchestrator func() *workflow.Orchestrator, cloudClient func() *cloud.Client, wsHub *WSHub, taskRoot string) *TasksHandler {
	return &TasksHandler{
		sessionDB:    sessionDB,
		orchestrator: orchestrator,
		cloudClient:  cloudClient,
		wsHub:        wsHub,
		taskRoot:     taskRoot,
	}
}

// taskDB returns the application-scoped task database.
func (h *TasksHandler) taskDB() *sql.DB {
	if h.sessionDB == nil {
		return nil
	}
	return h.sessionDB()
}

// taskCloudClient returns the cloud client used by the current session (login target: official or custom address).
// When logging in with a custom address, the session holds an independent client; if the session/client is missing, it falls back to the default client held by the processor.
// taskSyncPusher pushes local task execution results to the cloud incrementally via REST.
var taskSyncPusher = cloudsync.DefaultPusher()

// pushTaskSync aggregates and pushes the task's latest increment in the background.
func (h *TasksHandler) pushTaskSync(taskUUID string) {
	if h.taskCloudClient() == nil || h.taskDB() == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := taskSyncPusher.PushTaskSync(ctx, h.taskDB(), h.taskCloudClient(), taskUUID); err != nil {
			applog.Warn("[CloudSync] 推送任务结果失败", "task_uuid", taskUUID, "error", err.Error())
		}
	}()
}

// notifyTaskChanged asks every open task view to reload the task snapshot and
// progress. The browser filters by task_uuid, so unrelated task pages do not reload.
func (h *TasksHandler) notifyTaskChanged(taskUUID string) {
	if h.wsHub == nil {
		return
	}
	h.wsHub.BroadcastTaskChanged(taskUUID)
}

// deleteTaskSync removes the cloud projection of a locally deleted task in the background.
func (h *TasksHandler) deleteTaskSync(taskUUID string) {
	if h.taskCloudClient() == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := taskSyncPusher.DeleteTaskSync(ctx, h.taskCloudClient(), taskUUID); err != nil {
			applog.Warn("[CloudSync] 删除云端任务投影失败", "task_uuid", taskUUID, "error", err.Error())
		}
	}()
}
func (h *TasksHandler) taskCloudClient() *cloud.Client {
	if h.cloudClient == nil {
		return nil
	}
	return h.cloudClient()
}

// RegisterRoutes registers task routes
func (h *TasksHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.listTasks)
	r.GET("/:uuid", h.getTask)
	r.POST("", h.createTask)
	r.PUT("/:uuid/status", h.updateTaskStatus)
	r.POST("/:uuid/start", h.startTask)
	r.POST("/:uuid/assign-pipeline", h.assignAndStartTask)
	r.PUT("/:uuid/pipeline", h.assignTaskPipeline)
	r.GET("/:uuid/progress", h.listTaskProgress)
	r.POST("/:uuid/steps/:step_uuid/questions", h.askStepQuestion)
	r.POST("/:uuid/steps/:step_uuid/complete", h.completeStep)
	r.POST("/:uuid/steps/:step_uuid/runs", h.runStep)
	r.PUT("/:uuid/steps/:step_uuid/prompt", h.updateStepPrompt)
	r.GET("/:uuid/files", h.listTaskFiles)
	r.GET("/:uuid/files/content", h.getTaskFileContent)
	r.POST("/:uuid/files/content", h.saveTaskFileContent)
	r.GET("/:uuid/sessions", h.listTaskSessions)
	r.POST("/sessions/:uuid/messages", h.continueSession)
	r.POST("/sessions/:uuid/stop", h.stopSession)
	r.DELETE("/:uuid", h.deleteTask)

	r.GET("/cli-count", h.getCliTaskCount)
	r.GET("/cli-discovery", h.cliDiscovery)
	r.GET("/cli-models", h.cliDiscoveryModels)

	// Status (kanban column) configuration
	r.GET("/task-lanes", h.listTaskLanes)
	r.POST("/task-lanes", h.createTaskLane)
	r.PUT("/task-lanes/reorder", h.reorderTaskLanes)
	r.PUT("/task-lanes/:id", h.updateTaskLane)
	r.DELETE("/task-lanes/:id", h.deleteTaskLane)
}

type stepExecutionConfig struct {
	StepUUID     string `json:"step_uuid"`
	SourceStepID string `json:"source_step_id"`
	CloudStepID  string `json:"cloud_step_id"`
	CLIType      string `json:"cli_type"`
	Model        string `json:"model"`
	ModelName    string `json:"model_name"`
}

type pipelineAssignmentInput struct {
	PipelineUUID string                `json:"pipeline_uuid"`
	StepConfigs  []stepExecutionConfig `json:"step_configs"`
}

func (h *TasksHandler) snapshotFromPipeline(ctx context.Context, input pipelineAssignmentInput) (taskruntime.Snapshot, error) {
	svc := pipeline.NewService(h.taskDB())
	pipe, err := svc.Get(ctx, strings.TrimSpace(input.PipelineUUID))
	if errors.Is(err, pipeline.ErrNotFound) && h.taskCloudClient() != nil {
		// 重启后内存缓存为空且登录异步同步尚未完成：对云端流水线做一次冷缓存兜底同步再重试。
		// 兜底同步独立限时，避免云端慢响应把任务创建/启动请求拖住。
		applog.Warn("[LocalServer] 流水线未在本地缓存中找到，触发云端重同步后重试", "pipeline_uuid", input.PipelineUUID)
		syncCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if _, syncErr := syncCloudPipelines(syncCtx, svc, h.taskCloudClient()); syncErr != nil {
			applog.Warn("[LocalServer] 流水线兜底重同步失败", "pipeline_uuid", input.PipelineUUID, "error", syncErr.Error())
		} else {
			pipe, err = svc.Get(ctx, strings.TrimSpace(input.PipelineUUID))
		}
	}
	if err != nil {
		return taskruntime.Snapshot{}, fmt.Errorf("读取流水线失败: %w", err)
	}
	configs := make(map[string]stepExecutionConfig, len(input.StepConfigs)*3)
	for _, config := range input.StepConfigs {
		for _, key := range []string{config.StepUUID, config.SourceStepID, config.CloudStepID} {
			if key = strings.TrimSpace(key); key != "" {
				configs[key] = config
			}
		}
	}
	sourceID := pipe.UUID
	if pipe.SourceType == "cloud" {
		sourceID = pipe.CloudPipelineID
	}
	snapshot := taskruntime.Snapshot{SourceType: pipe.SourceType, SourcePipelineID: sourceID,
		CloudSourceKey: pipe.CloudSourceKey, Name: pipe.Name, Description: pipe.Description, Avatar: pipe.Avatar}
	for _, step := range pipe.Steps {
		config, ok := configs[step.UUID]
		if !ok && step.CloudStepID != "" {
			config = configs[step.CloudStepID]
		}
		cliType := strings.TrimSpace(config.CLIType)
		if cliType == "" {
			cliType = strings.TrimSpace(step.CLIType)
		}
		model := strings.TrimSpace(config.ModelName)
		if model == "" {
			model = strings.TrimSpace(config.Model)
		}
		if model == "" {
			model = strings.TrimSpace(step.ModelName)
		}
		snapshotStep := taskruntime.SnapshotStep{SourceStepID: step.UUID, LocalStepID: step.UUID,
			Name: step.Name, Description: step.Description, Avatar: step.Avatar, Prompt: step.Prompt,
			CLIType: cliType, ModelName: model, SortOrder: step.SortOrder}
		if pipe.SourceType == "cloud" {
			snapshotStep.SourceStepID = step.CloudStepID
			snapshotStep.LocalStepID = ""
			snapshotStep.CloudStepID = step.CloudStepID
		}
		snapshot.Steps = append(snapshot.Steps, snapshotStep)
	}
	return snapshot, nil
}

// RegisterTeamRoutes registers the only task APIs that require a cloud login.
func (h *TasksHandler) RegisterTeamRoutes(r *gin.RouterGroup) {
	r.GET("/my-work", h.listMyWork)
	r.GET("/pipelines", h.listTeamPipelines)
	r.GET("/pipelines/:id/snapshot", h.getTeamPipelineSnapshot)
	r.POST("/imports", h.importTeamTask)
}

func (h *TasksHandler) listTeamPipelines(c *gin.Context) {
	pipelines, err := h.taskCloudClient().GetPipelines(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	items := make([]gin.H, 0, len(pipelines))
	for _, item := range pipelines {
		items = append(items, gin.H{"id": item.ID, "name": item.Name, "description": "", "step_count": 0})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *TasksHandler) getTeamPipelineSnapshot(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "云端流水线 ID 无效"})
		return
	}
	snapshot, err := h.taskCloudClient().GetPipelineSnapshot(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	steps := make([]gin.H, 0, len(snapshot.Steps))
	for _, step := range snapshot.Steps {
		steps = append(steps, gin.H{"id": step.StepKey, "uuid": step.StepKey, "name": step.Name, "sort_order": step.SortOrder})
	}
	c.JSON(http.StatusOK, gin.H{"id": snapshot.ID, "name": snapshot.Name, "description": "", "steps": steps})
}

func (h *TasksHandler) importTeamTask(c *gin.Context) {
	var body struct {
		WorkItemType  string          `json:"work_item_type"`
		WorkItemID    json.RawMessage `json:"work_item_id"`
		CloudPipeline json.RawMessage `json:"cloud_pipeline_id"`
		StepConfigs   []struct {
			CloudStepID string `json:"cloud_step_id"`
			CLIType     string `json:"cli_type"`
			Model       string `json:"model"`
		} `json:"step_configs"`
		WorkDir         string   `json:"work_dir"`
		WorkDirs        []string `json:"work_dirs"`
		ProjectUUID     string   `json:"project_uuid"`
		SubprojectUUIDs []string `json:"subproject_uuids"`
		Status          string   `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	parseID := func(raw json.RawMessage) (int64, error) {
		value := strings.Trim(string(raw), "\"")
		return strconv.ParseInt(value, 10, 64)
	}
	workItemID, err := parseID(body.WorkItemID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "工作项 ID 无效"})
		return
	}
	pipelineID, err := parseID(body.CloudPipeline)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "云端流水线 ID 无效"})
		return
	}
	items, err := h.taskCloudClient().GetWorkItems(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	var selected *cloud.WorkItem
	for i := range items {
		if items[i].Type == body.WorkItemType && items[i].ID == workItemID {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "云端工作项不存在"})
		return
	}
	applog.Info("[CloudSync] 导入团队任务：已从云端工作项列表匹配到 selected",
		"work_item_id", selected.ID, "work_item_type", selected.Type, "workspace_id", selected.WorkspaceID)
	remote, err := h.taskCloudClient().GetPipelineSnapshot(c.Request.Context(), pipelineID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	workDirs := body.WorkDirs
	if len(workDirs) == 0 && strings.TrimSpace(body.WorkDir) != "" {
		workDirs = []string{body.WorkDir}
	}
	session := h.currentSessionForRequest(c)
	if session == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	cloudSourceKey := pipeline.CloudSourceKey(h.taskCloudClient().BaseURL())
	pipeSvc := pipeline.NewService(h.taskDB())
	existing, err := pipeSvc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cloudPipelines := make([]pipeline.CloudPipeline, 0)
	remoteID := strconv.FormatInt(pipelineID, 10)
	for _, item := range existing {
		if item.SourceType != "cloud" || item.CloudSourceKey != cloudSourceKey || item.CloudPipelineID == remoteID {
			continue
		}
		cached := pipeline.CloudPipeline{CloudPipelineID: item.CloudPipelineID, Name: item.Name, Description: item.Description, Avatar: item.Avatar}
		for _, step := range item.Steps {
			cached.Steps = append(cached.Steps, pipeline.CloudStep{CloudStepID: step.CloudStepID, SortOrder: step.SortOrder, Name: step.Name, Description: step.Description, Avatar: step.Avatar, Prompt: step.Prompt})
		}
		cloudPipelines = append(cloudPipelines, cached)
	}
	selectedCloud := pipeline.CloudPipeline{CloudPipelineID: remoteID, Name: remote.Name, Avatar: remote.Icon}
	for _, step := range remote.Steps {
		selectedCloud.Steps = append(selectedCloud.Steps, pipeline.CloudStep{CloudStepID: step.StepKey, SortOrder: step.SortOrder, Name: step.Name, Prompt: step.Prompt})
	}
	cloudPipelines = append(cloudPipelines, selectedCloud)
	synced, err := pipeSvc.SyncCloud(c.Request.Context(), cloudSourceKey, cloudPipelines)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	selectedPipelineUUID := ""
	for _, item := range synced {
		if item.CloudPipelineID == remoteID {
			selectedPipelineUUID = item.UUID
			break
		}
	}
	if selectedPipelineUUID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "云端流水线本地缓存失败"})
		return
	}
	configs := make([]stepExecutionConfig, 0, len(body.StepConfigs))
	for _, item := range body.StepConfigs {
		configs = append(configs, stepExecutionConfig{CloudStepID: item.CloudStepID, CLIType: item.CLIType, Model: item.Model})
	}
	// 待开始任务：先验证流水线快照结构，再创建任务。
	// 直接执行的任务需校验 CLI 和模型；待开始任务只校验结构，CLI 和模型在启动时校验。
	if selectedPipelineUUID != "" && len(configs) > 0 {
		if preSnapshot, preErr := h.snapshotFromPipeline(c.Request.Context(), pipelineAssignmentInput{
			PipelineUUID: selectedPipelineUUID,
			StepConfigs:  configs,
		}); preErr == nil {
			var preErr error
			if body.Status == protocol.TaskStatusInProgress || body.Status == protocol.TaskStatusActive {
				preErr = taskruntime.ValidateSnapshotFull(&preSnapshot)
			} else {
				preErr = taskruntime.ValidateSnapshot(&preSnapshot)
			}
			if preErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": preErr.Error()})
				return
			}
		}
	}
	selectedConfig, _ := json.Marshal(body.StepConfigs)
	taskUUID, err := taskruntime.NewService(h.taskDB(), h.taskRoot).Create(c.Request.Context(), taskruntime.CreateInput{
		SourceType: "cloud", CloudSourceKey: cloudSourceKey, CloudUserID: session.UserID,
		WorkItemType: selected.Type, WorkItemID: strconv.FormatInt(selected.ID, 10), WorkspaceID: selected.WorkspaceID,
		Title: selected.Title, Content: selected.Description, WorkDirs: workDirs, ProjectUUID: body.ProjectUUID,
		SubprojectUUIDs: body.SubprojectUUIDs, Status: "pending", SelectedPipelineUUID: selectedPipelineUUID,
		SelectedPipelineConfigJSON: string(selectedConfig),
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if body.Status == protocol.TaskStatusInProgress || body.Status == protocol.TaskStatusActive {
		if err = h.ensureTaskPipelineAssigned(c.Request.Context(), taskUUID, pipelineAssignmentInput{PipelineUUID: selectedPipelineUUID, StepConfigs: configs}); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "uuid": taskUUID})
			return
		}
		if _, err = h.startTaskExecution(c.Request.Context(), taskUUID, ""); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "uuid": taskUUID})
			return
		}
	} else if len(configs) > 0 {
		// 待开始任务：分配流水线快照（CLI 和模型在 Start 时校验）
		if err = h.ensureTaskPipelineAssigned(c.Request.Context(), taskUUID, pipelineAssignmentInput{PipelineUUID: selectedPipelineUUID, StepConfigs: configs}); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "uuid": taskUUID})
			return
		}
	}
	// 导入完成后立即同步一次任务投影到云端，避免后续会话推送被云端以 404「任务不存在」拒绝。
	// 同步调用以确保返回 201 时云端已有任务记录；推送失败不阻塞导入响应，仅返回 warning。
	// 分支 C（无 configs）由于 pipeline_snapshot_uuid 为空，BuildTaskSync 会静默跳过，无副作用。
	warning := ""
	var pushSnapshotUUID string
	if h.taskDB() != nil {
		_ = h.taskDB().QueryRowContext(c.Request.Context(), `SELECT pipeline_snapshot_uuid FROM gt_tasks WHERE uuid = ?`, taskUUID).Scan(&pushSnapshotUUID)
	}
	applog.Info("[CloudSync] 导入后准备同步任务投影", "task_uuid", taskUUID,
		"pipeline_snapshot_uuid", pushSnapshotUUID, "cloud_client_nil", h.taskCloudClient() == nil)
	if pushSnapshotUUID == "" {
		applog.Warn("[CloudSync] 导入任务未分配流水线快照，本次不会推送云端；进入详情页分配流水线/步骤后，首次执行或状态变更时才会同步",
			"task_uuid", taskUUID)
	}
	if h.taskCloudClient() == nil || h.taskDB() == nil {
		applog.Warn("[CloudSync] 导入后未推送任务投影：云端客户端或任务数据库未初始化",
			"cloud_client_nil", h.taskCloudClient() == nil, "db_nil", h.taskDB() == nil, "task_uuid", taskUUID)
	} else {
		syncCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		if err := taskSyncPusher.PushTaskSync(syncCtx, h.taskDB(), h.taskCloudClient(), taskUUID); err != nil {
			applog.Warn("[CloudSync] 导入后推送任务投影失败", "task_uuid", taskUUID, "error", err.Error())
			warning = "任务已导入，但云端投影同步失败：" + err.Error()
		} else {
			applog.Info("[CloudSync] 导入后任务投影已推送云端", "task_uuid", taskUUID)
		}
		cancel()
	}
	resp := gin.H{"uuid": taskUUID}
	if warning != "" {
		resp["warning"] = warning
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *TasksHandler) currentSessionForRequest(c *gin.Context) *browserSession {
	session, ok := getSession(c)
	if !ok {
		return nil
	}
	return session
}

// listMyWork returns the "My Work" list in the cloud and marks the work items for which local tasks have been created.
func (h *TasksHandler) listMyWork(c *gin.Context) {
	session, ok := getSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	result, err := h.taskCloudClient().GetMyWork(c.Request.Context())
	if err != nil {
		var apiErr *cloud.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "云端登录已失效，请重新登录"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "获取我的工作失败: " + err.Error()})
		return
	}

	assigned := make(map[string]string)
	cloudSourceKey := pipeline.CloudSourceKey(h.taskCloudClient().BaseURL())
	rows, err := h.taskDB().Query(
		`SELECT work_item_type, work_item_id, uuid FROM gt_tasks WHERE source_type = 'cloud' AND cloud_source_key = ? AND cloud_user_id = ?`,
		cloudSourceKey, session.UserID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询工作项分配状态失败: " + err.Error()})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var workItemType, workItemID, taskUUID string
		if err := rows.Scan(&workItemType, &workItemID, &taskUUID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取工作项分配状态失败: " + err.Error()})
			return
		}
		assigned[workItemType+":"+workItemID] = taskUUID
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取工作项分配状态失败: " + err.Error()})
		return
	}

	annotateMyWorkItems(result.List, assigned)
	c.JSON(http.StatusOK, gin.H{"items": result.List})
}

func annotateMyWorkItems(items []map[string]interface{}, assigned map[string]string) {
	for _, item := range items {
		workItemType, _ := item["type"].(string)
		workItemID := workItemIDString(item["id"])
		if taskUUID := assigned[workItemType+":"+workItemID]; taskUUID != "" {
			item["local_task_uuid"] = taskUUID
		}
	}
}

func workItemIDString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

// getCliTaskCount returns the number of CLI tasks currently running globally.
func (h *TasksHandler) getCliTaskCount(c *gin.Context) {
	count := 0
	if orch := h.orchestrator(); orch != nil {
		if n, err := orch.GlobalRunningSessionCount(); err == nil {
			count = n
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// cliDiscovery returns the installed CLI list. It prefers the on-disk snapshot
// (clis.json, maintained by executor.StartCLIDiscoveryRefresh) and only falls
// back to live detection when the snapshot is missing, so the "选择执行 CLI"
// popup opens without spawning every CLI on each request.
func (h *TasksHandler) cliDiscovery(c *gin.Context) {
	items := executor.GetCLIDiscovery(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// cliDiscoveryModels loads the model list for a single CLI type. It prefers
// the on-disk snapshot (models.json); only when the snapshot is missing does
// it spawn the CLI to probe live.
func (h *TasksHandler) cliDiscoveryModels(c *gin.Context) {
	cliType := c.Query("cli_type")
	if cliType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cli_type 不能为空"})
		return
	}
	models := executor.GetCLIModels(c.Request.Context(), cliType)
	c.JSON(http.StatusOK, gin.H{"models": models})
}

// listTasks task list
func (h *TasksHandler) listTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	if status != "" {
		status = normalizeTaskStatus(status)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT t.uuid, t.title, t.status, t.execution_status, t.cloud_agent_id, t.work_item_type, t.work_item_id,
		t.created_at, t.updated_at, t.content_snapshot,
		t.cloud_agent_name_snapshot, t.project_uuid, COALESCE(pr.name,''), COALESCE(pr.icon,''), t.selected_pipeline_uuid,
		t.pipeline_snapshot_uuid,
		COALESCE(NULLIF(ps.name,''), sel.name, ''), COALESCE(NULLIF(ps.avatar,''), sel.avatar, ''), t.priority, t.planned_start_date, t.planned_end_date
		FROM gt_tasks t
		LEFT JOIN gt_projects pr ON pr.uuid=t.project_uuid
		LEFT JOIN gt_task_pipeline_snapshots ps ON ps.uuid=t.pipeline_snapshot_uuid
		LEFT JOIN gt_pipelines sel ON sel.uuid=t.selected_pipeline_uuid`
	var rows *sql.Rows
	var err error
	if status != "" {
		query += ` WHERE t.status = ? ORDER BY t.created_at DESC LIMIT ? OFFSET ?`
		rows, err = h.taskDB().Query(query, status, pageSize, offset)
	} else {
		query += ` ORDER BY t.created_at DESC LIMIT ? OFFSET ?`
		rows, err = h.taskDB().Query(query, pageSize, offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type TaskItem struct {
		UUID                   string `json:"uuid"`
		Title                  string `json:"title"`
		Status                 string `json:"status"`
		ExecutionStatus        string `json:"execution_status"`
		AgentID                string `json:"agent_id"`
		WorkItemType           string `json:"work_item_type"`
		WorkItemID             string `json:"work_item_id"`
		CreatedAt              int64  `json:"created_at"`
		UpdatedAt              int64  `json:"updated_at"`
		ContentSnapshot        string `json:"content_snapshot"`
		AgentNameSnapshot      string `json:"agent_name_snapshot"`
		ProjectUUID            string `json:"project_uuid"`
		ProjectName            string `json:"project_name"`
		ProjectIcon            string `json:"project_icon"`
		SelectedPipelineUUID   string `json:"selected_pipeline_uuid"`
		PipelineSnapshotUUID   string `json:"pipeline_snapshot_uuid"`
		PipelineNameSnapshot   string `json:"pipeline_name_snapshot"`
		PipelineAvatarSnapshot string `json:"pipeline_avatar_snapshot"`
		Priority               string `json:"priority"`
		PlannedStartDate       string `json:"planned_start_date"`
		PlannedEndDate         string `json:"planned_end_date"`
	}

	items := make([]TaskItem, 0)
	for rows.Next() {
		var t TaskItem
		// Scan must be aborted if it fails: ignoring the error will append the zero-valued TaskItem to the result set.
		//The front end gets the "ghost task" with an empty UUID.
		if err := rows.Scan(&t.UUID, &t.Title, &t.Status, &t.ExecutionStatus, &t.AgentID, &t.WorkItemType, &t.WorkItemID,
			&t.CreatedAt, &t.UpdatedAt, &t.ContentSnapshot, &t.AgentNameSnapshot, &t.ProjectUUID, &t.ProjectName, &t.ProjectIcon,
			&t.SelectedPipelineUUID, &t.PipelineSnapshotUUID, &t.PipelineNameSnapshot, &t.PipelineAvatarSnapshot, &t.Priority, &t.PlannedStartDate, &t.PlannedEndDate); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取任务列表失败: " + err.Error()})
			return
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "遍历任务列表失败: " + err.Error()})
		return
	}

	// 云端流水线仅存内存：快照未冻结时，用内存缓存补齐所选流水线的名称/头像展示。
	for index := range items {
		if items[index].PipelineNameSnapshot != "" || items[index].SelectedPipelineUUID == "" {
			continue
		}
		if pipe, ok := pipeline.DefaultStore().Get(items[index].SelectedPipelineUUID); ok {
			items[index].PipelineNameSnapshot = pipe.Name
			items[index].PipelineAvatarSnapshot = pipe.Avatar
		}
	}

	var total int64
	countQuery := `SELECT COUNT(*) FROM gt_tasks`
	if status != "" {
		countQuery += ` WHERE status = ?`
		err = h.taskDB().QueryRow(countQuery, status).Scan(&total)
	} else {
		err = h.taskDB().QueryRow(countQuery).Scan(&total)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计任务总数失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// getTask task details
func (h *TasksHandler) getTask(c *gin.Context) {
	task, err := h.loadTaskDetail(c.Request.Context(), c.Param("uuid"))
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// loadTaskDetail loads the complete task detail, including work dirs, related projects and steps.
func (h *TasksHandler) loadTaskDetail(ctx context.Context, taskUUID string) (map[string]interface{}, error) {
	var task map[string]interface{}
	var uuid, title, description, status, execStatus, sourceType, agentID, cliType, workItemType, workItemID, workDir string
	var taskDir, currentStepUUID, selectedPipelineUUID, pipelineSnapshotUUID, pipelineName, pipelineAvatar, projectUUID, projectName, projectIcon string
	var priority, plannedStartDate, plannedEndDate string
	var currentStepCompleted bool
	var createdAt, updatedAt int64
	err := h.taskDB().QueryRowContext(ctx,
		`SELECT t.uuid, t.title, t.content_snapshot, t.status, t.execution_status, t.source_type, t.cloud_agent_id, t.cli_type,
		        t.work_item_type, t.work_item_id, t.work_dir, t.task_dir, t.current_step_uuid, t.current_step_completed,
		        t.selected_pipeline_uuid, t.pipeline_snapshot_uuid, COALESCE(NULLIF(p.name,''), sel.name, ''),
		        COALESCE(NULLIF(p.avatar,''), sel.avatar, ''), t.project_uuid, COALESCE(pr.name,''), COALESCE(pr.icon,''),
		        t.priority, t.planned_start_date, t.planned_end_date, t.created_at, t.updated_at
		 FROM gt_tasks t LEFT JOIN gt_task_pipeline_snapshots p ON p.uuid = t.pipeline_snapshot_uuid
		 LEFT JOIN gt_pipelines sel ON sel.uuid=t.selected_pipeline_uuid LEFT JOIN gt_projects pr ON pr.uuid=t.project_uuid WHERE t.uuid = ?`,
		taskUUID).Scan(
		&uuid, &title, &description, &status, &execStatus, &sourceType, &agentID, &cliType, &workItemType, &workItemID,
		&workDir, &taskDir, &currentStepUUID, &currentStepCompleted, &selectedPipelineUUID, &pipelineSnapshotUUID, &pipelineName,
		&pipelineAvatar, &projectUUID, &projectName, &projectIcon, &priority, &plannedStartDate, &plannedEndDate, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	// 云端流水线仅存内存：快照未冻结时，用内存缓存补齐所选流水线的名称/头像展示。
	if pipelineName == "" && selectedPipelineUUID != "" {
		if pipe, ok := pipeline.DefaultStore().Get(selectedPipelineUUID); ok {
			pipelineName = pipe.Name
			pipelineAvatar = pipe.Avatar
		}
	}

	task = map[string]interface{}{
		"uuid":                     uuid,
		"title":                    title,
		"description":              description,
		"status":                   status,
		"execution_status":         execStatus,
		"source_type":              sourceType,
		"agent_id":                 agentID,
		"cli_type":                 cliType,
		"work_item_type":           workItemType,
		"work_item_id":             workItemID,
		"work_dir":                 workDir,
		"task_dir":                 taskDir,
		"current_step_uuid":        currentStepUUID,
		"current_step_completed":   currentStepCompleted,
		"pipeline_snapshot_uuid":   pipelineSnapshotUUID,
		"selected_pipeline_uuid":   selectedPipelineUUID,
		"pipeline_name_snapshot":   pipelineName,
		"pipeline_avatar_snapshot": pipelineAvatar,
		"priority":                 priority,
		"planned_start_date":       plannedStartDate,
		"planned_end_date":         plannedEndDate,
		"project_uuid":             projectUUID, "project_name": projectName, "project_icon": projectIcon,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}

	workDirs, err := h.loadTaskWorkDirs(taskUUID, workDir)
	if err != nil {
		return nil, err
	}
	task["work_dirs"] = workDirs
	projectRows, err := h.taskDB().QueryContext(ctx, `SELECT l.project_uuid, p.name, p.icon, p.main_dir, l.relation_type, l.sort_order
		FROM gt_task_project_links l JOIN gt_projects p ON p.uuid=l.project_uuid WHERE l.task_uuid=? ORDER BY l.sort_order, l.id`, taskUUID)
	if err != nil {
		return nil, err
	}
	projects := make([]gin.H, 0)
	for projectRows.Next() {
		var id, name, icon, mainDir, relation string
		var order int
		if err := projectRows.Scan(&id, &name, &icon, &mainDir, &relation, &order); err != nil {
			projectRows.Close()
			return nil, err
		}
		projects = append(projects, gin.H{"uuid": id, "name": name, "icon": icon, "icon_type": icon,
			"main_dir": mainDir, "local_dir": mainDir, "relation_type": relation, "sort_order": order})
	}
	projectRows.Close()
	if err := projectRows.Err(); err != nil {
		return nil, err
	}
	task["projects"] = projects

	// Query steps
	rows, err := h.taskDB().QueryContext(ctx,
		`SELECT uuid, step_key, name, description, avatar, step_order, cli_type, model_name, status, execution_status, prompt_snapshot, step_dir
		 FROM gt_task_steps WHERE task_uuid = ? ORDER BY step_order`,
		taskUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []map[string]interface{}
	for rows.Next() {
		var sUUID, stepKey, name, description, avatar, cliType, modelName, stepStatus, stepExecStatus, prompt, stepDir string
		var sortOrder int
		if err := rows.Scan(&sUUID, &stepKey, &name, &description, &avatar, &sortOrder, &cliType, &modelName, &stepStatus, &stepExecStatus, &prompt, &stepDir); err != nil {
			return nil, fmt.Errorf("读取任务步骤失败: %w", err)
		}
		steps = append(steps, map[string]interface{}{
			"uuid":             sUUID,
			"step_key":         stepKey,
			"name":             name,
			"description":      description,
			"avatar":           avatar,
			"sort_order":       sortOrder,
			"cli_type":         cliType,
			"model":            modelName,
			"model_name":       modelName,
			"status":           stepStatus,
			"execution_status": stepExecStatus,
			"prompt_snapshot":  prompt,
			"step_dir":         stepDir,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []map[string]interface{}{}
	}

	task["steps"] = steps
	return task, nil
}

func (h *TasksHandler) loadTaskWorkDirs(taskUUID, legacyWorkDir string) ([]string, error) {
	rows, err := h.taskDB().Query(
		`SELECT path FROM gt_task_work_dirs WHERE task_uuid = ? ORDER BY sort_order, id`,
		taskUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询任务工作目录失败: %w", err)
	}
	defer rows.Close()

	workDirs := make([]string, 0)
	for rows.Next() {
		var workDir string
		if err := rows.Scan(&workDir); err != nil {
			return nil, fmt.Errorf("读取任务工作目录失败: %w", err)
		}
		workDirs = append(workDirs, workDir)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务工作目录失败: %w", err)
	}
	if len(workDirs) == 0 && strings.TrimSpace(legacyWorkDir) != "" {
		workDirs = append(workDirs, legacyWorkDir)
	}
	return workDirs, nil
}

// createTask creates a task
func (h *TasksHandler) createTask(c *gin.Context) {
	var body struct {
		Title              string                `json:"title"`
		Description        string                `json:"description"`
		PipelineUUID       string                `json:"pipeline_uuid"`
		StepConfigs        []stepExecutionConfig `json:"step_configs"`
		ProjectUUID        string                `json:"project_uuid"`
		SubprojectUUIDs    []string              `json:"subproject_uuids"`
		ChildProjectUUIDs  []string              `json:"child_project_uuids"`
		WorkDir            string                `json:"work_dir"`
		WorkDirs           []string              `json:"work_dirs"`
		Priority           string                `json:"priority"`
		PlannedStartDate   string                `json:"planned_start_date"`
		PlannedEndDate     string                `json:"planned_end_date"`
		Status             string                `json:"status"`
		WorkItemType       string                `json:"work_item_type"`
		WorkItemID         json.RawMessage       `json:"work_item_id"`
		WorkspaceID        int64                 `json:"workspace_id"`
		CreateNotification bool                  `json:"create_notification"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sourceType := "local"
	var cloudSourceKey, cloudUserID string
	workItemID := strings.Trim(string(body.WorkItemID), "\"")
	if body.WorkItemType != "" && workItemID != "" {
		// 团队工作导入：复用新建任务表单，但保留云端工作项关联，
		// 使「我的工作」列表能标记该需求/缺陷为已导入。
		session := h.currentSessionForRequest(c)
		if session == nil || h.taskCloudClient() == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "导入云端工作项需要先登录"})
			return
		}
		sourceType = "cloud"
		cloudSourceKey = pipeline.CloudSourceKey(h.taskCloudClient().BaseURL())
		cloudUserID = session.UserID
		// 团队导入需携带工作项所属工作区 ID，否则云端校验 workspace_id>0 会拒绝创建任务投影，
		// 进而导致会话记录推送被 404「任务不存在」拒绝。前端创建表单未必传 workspace_id，此处从云端工作项列表补全。
		if body.WorkspaceID == 0 {
			if items, werr := h.taskCloudClient().GetWorkItems(c.Request.Context()); werr == nil {
				for i := range items {
					if items[i].Type == body.WorkItemType && strconv.FormatInt(items[i].ID, 10) == workItemID {
						body.WorkspaceID = items[i].WorkspaceID
						break
					}
				}
			} else {
				applog.Warn("[CloudSync] 团队导入补全 workspace_id 失败：拉取云端工作项列表出错", "work_item_id", workItemID, "error", werr.Error())
			}
			if body.WorkspaceID == 0 {
				applog.Warn("[CloudSync] 团队导入未获取到 workspace_id，云端将拒绝创建任务投影（会话推送会 404）", "work_item_id", workItemID, "work_item_type", body.WorkItemType)
			}
		}
	}

	rawWorkDirs := body.WorkDirs
	if len(rawWorkDirs) == 0 && strings.TrimSpace(body.WorkDir) != "" {
		rawWorkDirs = []string{body.WorkDir}
	}
	if len(body.SubprojectUUIDs) == 0 {
		body.SubprojectUUIDs = body.ChildProjectUUIDs
	}
	var err error
	// 通知创建路径会在创建任务前构建并校验完整快照，无需先重复读取流水线。
	// 其他路径保留原有的提前存在性检查，避免改变历史接口行为。
	if strings.TrimSpace(body.PipelineUUID) != "" && !body.CreateNotification {
		if _, err = pipeline.NewService(h.taskDB()).Get(c.Request.Context(), strings.TrimSpace(body.PipelineUUID)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	selectedConfig, err := json.Marshal(body.StepConfigs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	requestedStatus := normalizeTaskStatus(body.Status)
	var preparedSnapshot taskruntime.Snapshot
	preparedSnapshotReady := false
	if body.CreateNotification {
		if strings.TrimSpace(body.PipelineUUID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "创建任务通知需要先选择流水线", "code": "pipeline_required"})
			return
		}
		var buildErr error
		preparedSnapshot, buildErr = h.snapshotFromPipeline(c.Request.Context(), pipelineAssignmentInput{PipelineUUID: body.PipelineUUID, StepConfigs: body.StepConfigs})
		if buildErr != nil {
			status := http.StatusInternalServerError
			code := "pipeline_load_failed"
			if errors.Is(buildErr, pipeline.ErrNotFound) {
				status = http.StatusBadRequest
				code = "pipeline_not_found"
			}
			c.JSON(status, gin.H{"error": buildErr.Error(), "code": code})
			return
		}
		if validateErr := taskruntime.ValidateSnapshotFull(&preparedSnapshot); validateErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "流水线配置不完整: " + validateErr.Error(), "code": "pipeline_incomplete"})
			return
		}
		preparedSnapshotReady = true
		requestedStatus = "active"
	}
	initialStatus := requestedStatus
	if requestedStatus == "active" && strings.TrimSpace(body.PipelineUUID) == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "任务切换为进行中前必须选择流水线"})
		return
	}
	if initialStatus == "" || initialStatus == "active" {
		initialStatus = "pending"
	}
	taskUUID, err := taskruntime.NewService(h.taskDB(), h.taskRoot).Create(c.Request.Context(), taskruntime.CreateInput{
		SourceType: sourceType, CloudSourceKey: cloudSourceKey, CloudUserID: cloudUserID,
		WorkItemType: body.WorkItemType, WorkItemID: workItemID, WorkspaceID: body.WorkspaceID,
		Title: body.Title, Content: body.Description, WorkDirs: rawWorkDirs,
		ProjectUUID: body.ProjectUUID, SubprojectUUIDs: body.SubprojectUUIDs, Status: initialStatus,
		Priority: body.Priority, PlannedStartDate: body.PlannedStartDate, PlannedEndDate: body.PlannedEndDate,
		SelectedPipelineUUID: strings.TrimSpace(body.PipelineUUID), SelectedPipelineConfigJSON: string(selectedConfig),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "创建任务失败: " + err.Error()})
		return
	}
	sessionUUID := ""
	if requestedStatus == "active" {
		snapshot := preparedSnapshot
		if !preparedSnapshotReady {
			var buildErr error
			snapshot, buildErr = h.snapshotFromPipeline(c.Request.Context(), pipelineAssignmentInput{PipelineUUID: body.PipelineUUID, StepConfigs: body.StepConfigs})
			if buildErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": buildErr.Error(), "uuid": taskUUID})
				return
			}
		}
		if assignErr := taskruntime.NewService(h.taskDB(), h.taskRoot).AssignPipeline(c.Request.Context(), taskUUID, snapshot); assignErr != nil {
			c.JSON(http.StatusConflict, gin.H{"error": assignErr.Error(), "uuid": taskUUID})
			return
		}
		sessionUUID, err = h.startTaskExecution(c.Request.Context(), taskUUID, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "uuid": taskUUID})
			return
		}
	} else if strings.TrimSpace(body.PipelineUUID) != "" {
		// 新建任务选择了流水线但未立即启动：步骤 CLI/模型齐全时直接初始化执行副本（task_steps），
		// 步骤缺少本机配置的流水线不在此处生成快照，进入详情页后通过指定流水线引导弹窗补全。
		if snapshot, buildErr := h.snapshotFromPipeline(c.Request.Context(), pipelineAssignmentInput{PipelineUUID: body.PipelineUUID, StepConfigs: body.StepConfigs}); buildErr == nil {
			if assignErr := taskruntime.NewService(h.taskDB(), h.taskRoot).AssignPipeline(c.Request.Context(), taskUUID, snapshot); assignErr != nil {
				c.JSON(http.StatusConflict, gin.H{"error": assignErr.Error(), "uuid": taskUUID})
				return
			}
		}
	}
	if body.CreateNotification {
		var stepUUID, progressUUID string
		var stepOrder int
		if sessionUUID != "" {
			_ = h.taskDB().QueryRowContext(c.Request.Context(),
				`SELECT s.uuid, s.sort_order, cs.task_progress_uuid
				 FROM gt_cli_sessions cs
				 JOIN gt_task_steps s ON s.uuid = cs.step_uuid
				 WHERE cs.uuid = ?`, sessionUUID).Scan(&stepUUID, &stepOrder, &progressUUID)
		}
		if stepUUID == "" {
			_ = h.taskDB().QueryRowContext(c.Request.Context(),
				`SELECT uuid, sort_order FROM gt_task_steps
				 WHERE task_uuid = ? ORDER BY sort_order LIMIT 1`, taskUUID).Scan(&stepUUID, &stepOrder)
		}

		taskDetail, detailErr := h.loadTaskDetail(c.Request.Context(), taskUUID)
		if detailErr != nil {
			c.JSON(http.StatusCreated, gin.H{"uuid": taskUUID, "warning": "任务已创建，但读取任务详情失败: " + detailErr.Error()})
			return
		}
		taskTitle, _ := taskDetail["title"].(string)
		firstStepName := ""
		if len(preparedSnapshot.Steps) > 0 {
			firstStepName = preparedSnapshot.Steps[0].Name
		}

		now := time.Now().UnixMilli()
		notifUUID := uuid.New().String()
		notifSessionUUID := uuid.New().String()
		notifTitle := taskTitle
		if firstStepName != "" {
			notifTitle = taskTitle + " · " + firstStepName
		}
		_, insertErr := h.taskDB().ExecContext(c.Request.Context(),
			`INSERT INTO gt_task_notifications
			 (uuid, task_uuid, task_step_uuid, step_order, progress_uuid, session_uuid,
			  terminal_status, title, summary, is_read, is_archived, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, ?)`,
			notifUUID, taskUUID, stepUUID, stepOrder, progressUUID, notifSessionUUID,
			"running", notifTitle, "任务已创建并开始执行", now)
		if insertErr != nil {
			applog.Warn("[createTask] 写入创建通知失败", "task_uuid", taskUUID, "error", insertErr.Error())
		}

		taskDetail["notification"] = buildTaskCreatedNotification(taskUUID, taskTitle, firstStepName, sessionUUID)
		c.JSON(http.StatusCreated, taskDetail)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"uuid": taskUUID})
}

// buildTaskCreatedNotification builds the "task created and started" notification
// carried in the create-task response. It is returned in memory only and not persisted.
func buildTaskCreatedNotification(taskUUID, taskTitle, firstStepName, sessionUUID string) gin.H {
	return gin.H{
		"task_uuid":    taskUUID,
		"task_title":   taskTitle,
		"step_name":    firstStepName,
		"session_uuid": sessionUUID,
		"status":       "created",
		"summary":      "任务已创建并开始执行",
		"created_at":   time.Now().UnixMilli(),
	}
}

// updateTaskStatus updates task status
func (h *TasksHandler) updateTaskStatus(c *gin.Context) {
	taskUUID := c.Param("uuid")
	var body struct {
		Status       string                `json:"status"`
		PipelineUUID string                `json:"pipeline_uuid"`
		StepConfigs  []stepExecutionConfig `json:"step_configs"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	body.Status = normalizeTaskStatus(body.Status)
	var previousStatus, currentStepUUID string
	if err := h.taskDB().QueryRow(`SELECT status, current_step_uuid FROM gt_tasks WHERE uuid = ?`, taskUUID).Scan(&previousStatus, &currentStepUUID); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if body.Status == "active" && previousStatus == "pending" && currentStepUUID == "" {
		if assignErr := h.ensureTaskPipelineAssigned(c.Request.Context(), taskUUID, pipelineAssignmentInput{PipelineUUID: body.PipelineUUID, StepConfigs: body.StepConfigs}); assignErr != nil {
			c.JSON(http.StatusConflict, gin.H{"error": assignErr.Error()})
			return
		}
		sessionUUID, err := h.startTaskExecution(c.Request.Context(), taskUUID, "")
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "in_progress", "session_uuid": sessionUUID})
		return
	}
	// Verify status value: first check the gt_task_lanes table, while being compatible with hard-coded status
	var laneCount int
	h.taskDB().QueryRow(`SELECT COUNT(*) FROM gt_task_lanes WHERE lane_key = ?`, body.Status).Scan(&laneCount)
	if laneCount == 0 && !workflow.ValidateTaskStatus(body.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务状态"})
		return
	}

	now := time.Now().UnixMilli()
	result, err := h.taskDB().Exec(`UPDATE gt_tasks SET status = ?, updated_at = ? WHERE uuid = ?`, body.Status, now, taskUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取更新影响行数失败: " + err.Error()})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if body.Status == "done" {
		_ = markTaskNotificationsRead(c.Request.Context(), h.taskDB(), taskUUID, true)
	}
	h.pushTaskSync(taskUUID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func normalizeTaskStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "", "pending", "todo":
		return "pending"
	case "active", "in_progress", "developing":
		return "active"
	case "done", "completed", "developed":
		return "done"
	case "blocked":
		return "blocked"
	default:
		return strings.TrimSpace(status)
	}
}

func (h *TasksHandler) startTask(c *gin.Context) {
	var input pipelineAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.ensureTaskPipelineAssigned(c.Request.Context(), c.Param("uuid"), input); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	sessionUUID, err := h.startTaskExecution(c.Request.Context(), c.Param("uuid"), c.GetHeader("X-Request-ID"))
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, taskruntime.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	h.pushTaskSync(c.Param("uuid"))
	c.JSON(http.StatusAccepted, gin.H{"session_uuid": sessionUUID})
}

func (h *TasksHandler) ensureTaskPipelineAssigned(ctx context.Context, taskUUID string, input pipelineAssignmentInput) error {
	var snapshotUUID, selectedPipelineUUID, configJSON string
	if err := h.taskDB().QueryRowContext(ctx, `SELECT pipeline_snapshot_uuid, selected_pipeline_uuid, selected_pipeline_config_json
		FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(&snapshotUUID, &selectedPipelineUUID, &configJSON); err == sql.ErrNoRows {
		return taskruntime.ErrNotFound
	} else if err != nil {
		return err
	}
	if snapshotUUID != "" {
		if len(input.StepConfigs) > 0 {
			for _, cfg := range input.StepConfigs {
				cliType := strings.TrimSpace(cfg.CLIType)
				model := strings.TrimSpace(cfg.Model)
				if model == "" {
					model = strings.TrimSpace(cfg.ModelName)
				}
				if cliType == "" && model == "" {
					continue
				}
				ids := []string{cfg.SourceStepID}
				if cfg.StepUUID != "" {
					ids = append(ids, cfg.StepUUID)
				}
				if cfg.CloudStepID != "" {
					ids = append(ids, cfg.CloudStepID)
				}
				for _, id := range ids {
					if id == "" {
						continue
					}
					h.taskDB().ExecContext(ctx,
						`UPDATE gt_task_steps SET cli_type=?, model_name=?, updated_at=? WHERE task_uuid=? AND (source_step_id=? OR cloud_step_id=?)`,
						cliType, model, time.Now().UnixMilli(), taskUUID, id, id)
				}
			}
		}
		return nil
	}
	if strings.TrimSpace(input.PipelineUUID) == "" {
		input.PipelineUUID = selectedPipelineUUID
	}
	if len(input.StepConfigs) == 0 && strings.TrimSpace(configJSON) != "" {
		if err := json.Unmarshal([]byte(configJSON), &input.StepConfigs); err != nil {
			return fmt.Errorf("读取预选流水线运行配置失败: %w", err)
		}
	}
	if strings.TrimSpace(input.PipelineUUID) == "" {
		return fmt.Errorf("任务启动前必须选择流水线")
	}
	configRaw, err := json.Marshal(input.StepConfigs)
	if err != nil {
		return err
	}
	if _, err = h.taskDB().ExecContext(ctx, `UPDATE gt_tasks SET selected_pipeline_uuid=?, selected_pipeline_config_json=?, updated_at=? WHERE uuid=?`,
		strings.TrimSpace(input.PipelineUUID), string(configRaw), time.Now().UnixMilli(), taskUUID); err != nil {
		return err
	}
	snapshot, err := h.snapshotFromPipeline(ctx, input)
	if err != nil {
		return err
	}
	return taskruntime.NewService(h.taskDB(), h.taskRoot).AssignPipeline(ctx, taskUUID, snapshot)
}

func (h *TasksHandler) assignTaskPipeline(c *gin.Context) {
	var input pipelineAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.PipelineUUID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pipeline_uuid 不能为空"})
		return
	}
	if err := h.ensureTaskPipelineAssigned(c.Request.Context(), c.Param("uuid"), input); err != nil {
		status := http.StatusConflict
		if errors.Is(err, taskruntime.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"assigned": true})
}

// assignAndStartTask freezes the selected pipeline into the task and starts its
// first step in one API operation. The snapshot remains permanent even if the
// source pipeline is later edited or deleted.
func (h *TasksHandler) assignAndStartTask(c *gin.Context) {
	var input pipelineAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.PipelineUUID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pipeline_uuid 不能为空"})
		return
	}
	if err := h.ensureTaskPipelineAssigned(c.Request.Context(), c.Param("uuid"), input); err != nil {
		status := http.StatusConflict
		if errors.Is(err, taskruntime.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	sessionUUID, err := h.startTaskExecution(c.Request.Context(), c.Param("uuid"), c.GetHeader("X-Request-ID"))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	h.pushTaskSync(c.Param("uuid"))
	c.JSON(http.StatusAccepted, gin.H{"assigned": true, "session_uuid": sessionUUID})
}

func (h *TasksHandler) startTaskExecution(ctx context.Context, taskUUID, requestID string) (string, error) {
	stepUUID, err := taskruntime.NewService(h.taskDB(), h.taskRoot).Start(ctx, taskUUID)
	if err != nil {
		return "", err
	}
	return h.orchestrator().RunStep(ctx, workflow.RunStepOptions{TaskUUID: taskUUID, StepUUID: stepUUID,
		RequestID: requestID, RecordType: "initial_run"})
}

func (h *TasksHandler) askStepQuestion(c *gin.Context) {
	var body struct {
		Question  string `json:"question"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "问题内容不能为空"})
		return
	}
	var parentSessionUUID string
	err := h.taskDB().QueryRowContext(c.Request.Context(), `SELECT uuid FROM gt_cli_sessions
		WHERE task_uuid = ? AND step_uuid = ? AND status IN ('success','failed','stopped','interrupted')
		ORDER BY run_no DESC LIMIT 1`, c.Param("uuid"), c.Param("step_uuid")).Scan(&parentSessionUUID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusConflict, gin.H{"error": "当前 Agent 编排尚无可继续的 CLI 会话"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sessionUUID, err := h.orchestrator().ContinueConversation(c.Request.Context(), workflow.ContinueConversationOptions{
		ParentSessionUUID: parentSessionUUID, Prompt: body.Question, RequestID: body.RequestID,
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	_ = markTaskNotificationsRead(c.Request.Context(), h.taskDB(), c.Param("uuid"), true)
	h.pushTaskSync(c.Param("uuid"))
	c.JSON(http.StatusAccepted, gin.H{"session_uuid": sessionUUID})
}

func (h *TasksHandler) completeStep(c *gin.Context) {
	result, err := taskruntime.NewService(h.taskDB(), h.taskRoot).CompleteStep(c.Request.Context(), c.Param("uuid"), c.Param("step_uuid"))
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, taskruntime.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	_ = markTaskNotificationsRead(c.Request.Context(), h.taskDB(), c.Param("uuid"), true)
	h.pushTaskSync(c.Param("uuid"))
	h.notifyTaskChanged(c.Param("uuid"))
	response := gin.H{"task_done": result.TaskDone, "next_step_uuid": result.NextStepUUID}
	if result.NextStepUUID != "" {
		sessionUUID, runErr := h.orchestrator().RunStep(c.Request.Context(), workflow.RunStepOptions{
			TaskUUID: c.Param("uuid"), StepUUID: result.NextStepUUID, RecordType: "initial_run",
		})
		if runErr != nil {
			response["start_error"] = runErr.Error()
		} else {
			response["session_uuid"] = sessionUUID
		}
	}
	c.JSON(http.StatusOK, response)
}

func (h *TasksHandler) listTaskProgress(c *gin.Context) {
	rows, err := h.taskDB().QueryContext(c.Request.Context(), `SELECT p.uuid, p.task_uuid, p.task_step_uuid, p.session_uuid,
		p.record_type, p.user_prompt, p.cli_type, p.model_name, p.status, p.final_result,
		p.created_at, p.started_at, p.finished_at,
		COALESCE(s.latest_event_type, ''), COALESCE(s.latest_event_content, ''), COALESCE(s.latest_event_at, 0)
		FROM gt_task_progress p LEFT JOIN gt_cli_sessions s ON s.uuid = p.session_uuid
		WHERE p.task_uuid = ? ORDER BY p.created_at, p.rowid`, c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id, taskID, stepID, sessionID, recordType, prompt, cliType, model, status, result string
		var createdAt, startedAt, finishedAt int64
		var latestEventType, latestEventContent string
		var latestEventAt int64
		if err := rows.Scan(&id, &taskID, &stepID, &sessionID, &recordType, &prompt, &cliType, &model, &status, &result, &createdAt, &startedAt, &finishedAt, &latestEventType, &latestEventContent, &latestEventAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, gin.H{"uuid": id, "task_uuid": taskID, "task_step_uuid": stepID,
			"session_uuid": sessionID, "record_type": recordType, "user_prompt": prompt, "question": prompt,
			"cli_type": cliType, "model": model, "model_name": model, "status": status,
			"final_result": result, "result": result, "created_at": createdAt, "started_at": startedAt, "finished_at": finishedAt,
			"latest_event_type": latestEventType, "latest_event_content": latestEventContent, "latest_event_at": latestEventAt})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// deleteTask delete task (cascade deletion of sub-table data)
func (h *TasksHandler) deleteTask(c *gin.Context) {
	taskUUID := c.Param("uuid")

	//Refuse to delete tasks with active Sessions to prevent the CLI process from becoming an orphan that cannot be terminated.
	// If the query fails, an error must be reported directly: swallowing the error will make activeCount always 0, and the protection will be in vain.
	// Instead, cascading deletions of running session records are performed - exactly what this check is intended to prevent.
	var activeCount int
	var taskDir string
	if err := h.taskDB().QueryRow(
		`SELECT task_dir, (SELECT COUNT(*) FROM gt_cli_sessions WHERE task_uuid = gt_tasks.uuid AND status IN ('created', 'running', 'waiting_input', 'stop_requested')) FROM gt_tasks WHERE uuid = ?`,
		taskUUID).Scan(&taskDir, &activeCount); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查任务活动会话失败: " + err.Error()})
		return
	}
	if activeCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "任务仍有正在执行的 CLI 会话，请先终止后再删除"})
		return
	}

	tx, err := h.taskDB().Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()

	// First delete the subtable that references the task uuid, and then delete the main task table to avoid foreign key constraint errors.
	if _, err := tx.Exec(`DELETE FROM gt_task_notifications WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务通知失败: " + err.Error()})
		return
	}
	if _, err := tx.Exec(`DELETE FROM gt_cli_sessions WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务会话失败: " + err.Error()})
		return
	}
	if _, err := tx.Exec(`DELETE FROM gt_task_progress WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务进度失败: " + err.Error()})
		return
	}
	if _, err := tx.Exec(`DELETE FROM gt_task_steps WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务步骤失败: " + err.Error()})
		return
	}
	if _, err := tx.Exec(`DELETE FROM gt_task_work_dirs WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务工作目录失败: " + err.Error()})
		return
	}
	if _, err := tx.Exec(`DELETE FROM gt_task_pipeline_snapshots WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除临时流水线失败: " + err.Error()})
		return
	}
	result, err := tx.Exec(`DELETE FROM gt_tasks WHERE uuid = ?`, taskUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务失败: " + err.Error()})
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取删除影响行数失败: " + err.Error()})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}
	root, _ := filepath.Abs(h.taskRoot)
	target, _ := filepath.Abs(taskDir)
	if rel, relErr := filepath.Rel(root, target); relErr == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		if err := os.RemoveAll(target); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库数据已删除，但任务专属目录删除失败: " + err.Error()})
			return
		}
	}

	h.deleteTaskSync(taskUUID)
	c.JSON(http.StatusOK, gin.H{"uuid": taskUUID, "deleted": true})
}

// runStep execution step
func (h *TasksHandler) runStep(c *gin.Context) {
	taskUUID := c.Param("uuid")
	stepUUID := c.Param("step_uuid")

	var body struct {
		RequestID string `json:"request_id"`
		CLIType   string `json:"cli_type"`
		Model     string `json:"model"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var storedWorkDir string
	err := h.taskDB().QueryRow(`SELECT work_dir FROM gt_tasks WHERE uuid = ?`, taskUUID).Scan(&storedWorkDir)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取任务工作目录失败: " + err.Error()})
		return
	}
	workDir, err := normalizeExistingDirectory(storedWorkDir)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务工作目录不可用: " + err.Error()})
		return
	}

	sessionUUID, err := h.orchestrator().RunStep(c.Request.Context(), workflow.RunStepOptions{
		TaskUUID:  taskUUID,
		StepUUID:  stepUUID,
		WorkDir:   workDir,
		RequestID: body.RequestID,
		CLIType:   body.CLIType,
		Model:     body.Model,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.pushTaskSync(taskUUID)
	c.JSON(http.StatusAccepted, gin.H{"session_uuid": sessionUUID})
}

// updateStepPrompt saves the prompt snapshot and invalidates the execution
// progress from this step onward. A step that had already entered execution is
// restarted with a fresh CLI round; a future, never-executed step remains
// pending and does not interrupt the current step.
func (h *TasksHandler) updateStepPrompt(c *gin.Context) {
	taskUUID := c.Param("uuid")
	stepUUID := c.Param("step_uuid")

	var body struct {
		PromptSnapshot string `json:"prompt_snapshot"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.PromptSnapshot) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt_snapshot 不能为空"})
		return
	}

	// 1. Verify task and step exist, and capture the state before resetting it.
	var stepStatus, stepExecutionStatus string
	var stepOrder, currentStepOrder int
	var taskExists bool
	err := h.taskDB().QueryRow(`SELECT COUNT(*)>0 FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(&taskExists)
	if err != nil || !taskExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	err = h.taskDB().QueryRow(
		`SELECT s.step_order, s.status, s.execution_status
		 FROM gt_task_steps s WHERE s.uuid=? AND s.task_uuid=?`,
		stepUUID, taskUUID,
	).Scan(&stepOrder, &stepStatus, &stepExecutionStatus)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "步骤不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询步骤失败: " + err.Error()})
		return
	}
	if err = h.taskDB().QueryRow(`SELECT COALESCE((
		SELECT current_step.step_order
		FROM gt_task_steps current_step
		WHERE current_step.uuid = t.current_step_uuid AND current_step.task_uuid = t.uuid
	), -1) FROM gt_tasks t WHERE t.uuid=?`, taskUUID).Scan(&currentStepOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询当前步骤失败: " + err.Error()})
		return
	}

	now := time.Now().UnixMilli()

	// 客户端断开（curl 超时、页面关闭）会取消请求 context，导致进程终止、
	// 状态重置等关键操作中途失败，留下「步骤已回退但无会话执行」的中间态；
	// 因此本接口内部一律使用不随客户端断开取消的 context。
	reqCtx := context.WithoutCancel(c.Request.Context())
	applog.Info("[updateStepPrompt] 开始修改步骤提示词", "task_uuid", taskUUID, "step_uuid", stepUUID)

	// active/completed are the business states for a step that has entered the
	// execution chain. execution_status also covers historical failed/stopped
	// attempts that may have already been rolled back to pending.
	affectedContainsCurrent := currentStepOrder >= stepOrder
	shouldAutoStart := affectedContainsCurrent &&
		(stepStatus == "active" || stepStatus == "completed" || stepExecutionStatus != "idle")

	// 2. When the changed range contains the current step, stop the active CLI
	// before deleting its progress. Editing a future pending step must not stop
	// the earlier current step.
	// 停止失败时不能继续：残留的 stop_requested 会话会阻塞后续会话启动，
	// 若继续修改步骤状态会出现「步骤已回退但无会话执行」的卡死状态，故直接返回让用户重试。
	if orch := h.orchestrator(); orch != nil && affectedContainsCurrent {
		stopStart := time.Now()
		if stopErr := orch.StopTaskSessions(reqCtx, taskUUID); stopErr != nil {
			applog.Warn("[updateStepPrompt] 终止任务会话失败", "task_uuid", taskUUID, "error", stopErr, "cost_ms", time.Since(stopStart).Milliseconds())
			c.JSON(http.StatusConflict, gin.H{"error": "终止当前执行失败，步骤提示词未修改，请稍后重试: " + stopErr.Error()})
			return
		}
		applog.Info("[updateStepPrompt] 终止任务会话完成", "task_uuid", taskUUID, "cost_ms", time.Since(stopStart).Milliseconds())
		// Wait briefly for sessions to fully terminate
		time.Sleep(500 * time.Millisecond)
	}

	tx, err := h.taskDB().Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()

	// 3. Clear only the progress belonging to the modified step and subsequent
	// steps. Earlier progress is still valid and must not be deleted.
	if _, err = tx.Exec(`DELETE FROM gt_task_progress WHERE task_uuid=? AND task_step_uuid IN (
		SELECT uuid FROM gt_task_steps WHERE task_uuid=? AND step_order>=?)`,
		taskUUID, taskUUID, stepOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清除步骤进度失败: " + err.Error()})
		return
	}

	// 4. Reset the changed step and every later step to a genuine unexecuted
	// state. Old CLI session rows are retained for audit, but no longer affect
	// the visible step state or the newly-created execution round.
	if _, err = tx.Exec(`UPDATE gt_task_steps SET
		status='pending', execution_status='idle',
		output_summary='', error_summary='',
		input_tokens=0, output_tokens=0, total_tokens=0, conversation_rounds=0,
		started_at=0, completed_at=0, finished_at=0, duration_ms=0,
		updated_at=?
		WHERE task_uuid=? AND step_order>=?`, now, taskUUID, stepOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置步骤状态失败: " + err.Error()})
		return
	}
	if _, err = tx.Exec(`UPDATE gt_task_steps SET prompt_snapshot=?, updated_at=? WHERE uuid=?`,
		body.PromptSnapshot, now, stepUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新提示词快照失败: " + err.Error()})
		return
	}

	// 5. A previously active/completed/executed step is immediately made current
	// and started after the transaction. A future idle step remains pending.
	if shouldAutoStart {
		if _, err = tx.Exec(`UPDATE gt_task_steps SET status='active', updated_at=? WHERE uuid=?`,
			now, stepUUID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "激活修改步骤失败: " + err.Error()})
			return
		}
		if _, err = tx.Exec(`UPDATE gt_tasks SET current_step_uuid=?, current_step_completed=0,
			execution_status='idle', status='active', finished_at=0, updated_at=? WHERE uuid=?`,
			stepUUID, now, taskUUID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "重置任务状态失败: " + err.Error()})
			return
		}
	}

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}

	h.pushTaskSync(taskUUID)
	h.notifyTaskChanged(taskUUID)
	if !shouldAutoStart {
		c.JSON(http.StatusOK, gin.H{"prompt_updated": true, "execution_deferred": true})
		return
	}

	// 6. Start a fresh round so the new prompt is used as the complete initial
	// prompt. Do not resume the old native conversation.
	sessionUUID, runErr := h.orchestrator().RunStep(reqCtx, workflow.RunStepOptions{
		TaskUUID: taskUUID,
		StepUUID: stepUUID,
	})
	if runErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"prompt_updated": true,
			"session_error":  runErr.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"prompt_updated": true,
		"session_uuid":   sessionUUID,
	})
}

// taskFileEntry represents a file or directory entry in the task file tree.
type taskFileEntry struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	IsDir    bool            `json:"is_dir"`
	FileType string          `json:"file_type,omitempty"`
	Size     int64           `json:"size,omitempty"`
	Children []taskFileEntry `json:"children,omitempty"`
}

// listTaskFiles returns the tree structure of files and folders under the task directory.
func (h *TasksHandler) listTaskFiles(c *gin.Context) {
	taskUUID := c.Param("uuid")

	var taskDir string
	err := h.taskDB().QueryRow(`SELECT task_dir FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(&taskDir)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败: " + err.Error()})
		return
	}
	if taskDir == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务目录为空"})
		return
	}

	root, err := filepath.Abs(taskDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "任务目录无效: " + err.Error()})
		return
	}

	tree, err := buildFileTree(root, root, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件树失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"task_dir": root, "tree": tree})
}

// buildFileTree recursively builds a file tree entry.
func buildFileTree(root, currentDir, relativePath string) ([]taskFileEntry, error) {
	entries, err := os.ReadDir(currentDir)
	if err != nil {
		return nil, err
	}

	result := make([]taskFileEntry, 0, len(entries))
	for _, entry := range entries {
		relPath := filepath.Join(relativePath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		node := taskFileEntry{
			Name:  entry.Name(),
			Path:  relPath,
			IsDir: entry.IsDir(),
		}
		if !entry.IsDir() {
			ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			node.FileType = ext
			node.Size = info.Size()
		} else {
			children, err := buildFileTree(root, filepath.Join(currentDir, entry.Name()), relPath)
			if err == nil && len(children) > 0 {
				node.Children = children
			} else {
				// Always include empty directories with empty children array
				node.Children = []taskFileEntry{}
			}
		}
		result = append(result, node)
	}
	return result, nil
}

// getTaskFileContent reads a file's content relative to the task directory.
func (h *TasksHandler) getTaskFileContent(c *gin.Context) {
	taskUUID := c.Param("uuid")
	filePath := c.Query("path")
	if strings.TrimSpace(filePath) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path 参数不能为空"})
		return
	}

	var taskDir string
	err := h.taskDB().QueryRow(`SELECT task_dir FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(&taskDir)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败: " + err.Error()})
		return
	}

	absPath, err := h.safeTaskFilePath(taskDir, filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Only allow text and image file types
	ext := strings.TrimPrefix(filepath.Ext(absPath), ".")
	if !isReadableFileType(ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持读取 %s 类型的文件内容，仅支持文本和图片文件", ext)})
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败: " + err.Error()})
		return
	}

	response := gin.H{
		"path":      filePath,
		"file_type": ext,
		"size":      len(content),
	}
	if isImageFileType(ext) {
		response["encoding"] = "base64"
		response["content"] = base64.StdEncoding.EncodeToString(content)
	} else {
		response["content"] = string(content)
	}
	c.JSON(http.StatusOK, response)
}

// saveTaskFileContent writes content to a file relative to the task directory.
func (h *TasksHandler) saveTaskFileContent(c *gin.Context) {
	taskUUID := c.Param("uuid")

	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path 不能为空"})
		return
	}

	var taskDir string
	err := h.taskDB().QueryRow(`SELECT task_dir FROM gt_tasks WHERE uuid=?`, taskUUID).Scan(&taskDir)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败: " + err.Error()})
		return
	}

	absPath, err := h.safeTaskFilePath(taskDir, body.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := os.WriteFile(absPath, []byte(body.Content), 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": body.Path, "saved": true})
}

// isReadableFileType checks if the file extension is a supported readable type (text or image).
func isReadableFileType(ext string) bool {
	return isTextFileType(ext) || isImageFileType(ext)
}

// isTextFileType checks if the file extension is a supported text-based type.
func isTextFileType(ext string) bool {
	switch strings.ToLower(ext) {
	case
		"md", "markdown",
		"txt", "text", "log",
		"json", "xml", "yaml", "yml", "toml", "ini", "cfg", "conf", "properties",
		"html", "htm", "css", "scss", "less", "js", "jsx", "ts", "tsx", "vue", "svelte",
		"go", "py", "java", "c", "cpp", "cc", "cxx", "h", "hpp", "hxx", "rs", "swift", "kt", "scala",
		"sh", "bash", "zsh", "fish", "ps1", "bat", "cmd",
		"sql", "graphql", "gql",
		"csv", "tsv",
		"dockerfile", "makefile", "gradle", "cmake",
		"gitignore", "gitkeep", "env", "editorconfig",
		"proto", "lock",
		"rst", "asciidoc", "adoc",
		"pl", "rb", "php", "lua", "r", "m", "mm",
		"dart", "ex", "exs",
		"zig", "nim", "crystal":
		return true
	default:
		return false
	}
}

// isImageFileType checks if the file extension is a supported image type.
func isImageFileType(ext string) bool {
	switch strings.ToLower(ext) {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "ico":
		return true
	default:
		return false
	}
}

// safeTaskFilePath validates and resolves a relative path within the task directory,
// preventing directory traversal attacks.
func (h *TasksHandler) safeTaskFilePath(taskDir, relPath string) (string, error) {
	if strings.TrimSpace(taskDir) == "" {
		return "", fmt.Errorf("任务目录为空")
	}
	root, err := filepath.Abs(strings.TrimSpace(taskDir))
	if err != nil {
		return "", fmt.Errorf("任务目录无效: %w", err)
	}
	cleanPath := filepath.Clean(strings.TrimSpace(relPath))
	if strings.Contains(cleanPath, "..") || filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("文件路径无效: %s", relPath)
	}
	absPath := filepath.Join(root, cleanPath)
	// Verify the resolved path is within the task directory
	absRoot, _ := filepath.Abs(root)
	absTarget, _ := filepath.Abs(absPath)
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("文件路径不在任务目录范围内")
	}
	return absPath, nil
}

// listTaskSessions returns all local CLI running records under the task, and the complete events can be played back on demand through WebSocket.
func (h *TasksHandler) listTaskSessions(c *gin.Context) {
	taskUUID := c.Param("uuid")
	rows, err := h.taskDB().Query(
		`SELECT s.uuid, s.task_uuid, s.step_uuid, ts.step_key, ts.name,
		        s.conversation_uuid, s.parent_session_uuid, s.run_no, s.status,
		        s.cli_type, s.external_session_id, s.work_dir, s.prompt_snapshot,
		        s.input_tokens, s.output_tokens, s.total_tokens,
		        s.started_at, s.finished_at, s.duration_ms, s.created_at, s.updated_at,
		        s.error_message
		 FROM gt_cli_sessions s
		 JOIN gt_task_steps ts ON ts.uuid = s.step_uuid
		 WHERE s.task_uuid = ?
		 ORDER BY ts.step_order ASC, s.created_at ASC`,
		taskUUID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type SessionItem struct {
		UUID              string `json:"uuid"`
		TaskUUID          string `json:"task_uuid"`
		StepUUID          string `json:"step_uuid"`
		StepKey           string `json:"step_key"`
		StepName          string `json:"step_name"`
		ConversationUUID  string `json:"conversation_uuid"`
		ParentSessionUUID string `json:"parent_session_uuid"`
		RunNo             int    `json:"run_no"`
		Status            string `json:"status"`
		CLIType           string `json:"cli_type"`
		ExternalSessionID string `json:"external_session_id"`
		WorkDir           string `json:"work_dir"`
		PromptSnapshot    string `json:"prompt_snapshot"`
		InputTokens       int    `json:"input_tokens"`
		OutputTokens      int    `json:"output_tokens"`
		TotalTokens       int    `json:"total_tokens"`
		StartedAt         int64  `json:"started_at"`
		FinishedAt        int64  `json:"finished_at"`
		DurationMs        int64  `json:"duration_ms"`
		CreatedAt         int64  `json:"created_at"`
		UpdatedAt         int64  `json:"updated_at"`
		ErrorMessage      string `json:"error_message"`
	}

	items := make([]SessionItem, 0)
	for rows.Next() {
		var item SessionItem
		if err := rows.Scan(
			&item.UUID, &item.TaskUUID, &item.StepUUID, &item.StepKey, &item.StepName,
			&item.ConversationUUID, &item.ParentSessionUUID, &item.RunNo, &item.Status,
			&item.CLIType, &item.ExternalSessionID, &item.WorkDir, &item.PromptSnapshot,
			&item.InputTokens, &item.OutputTokens, &item.TotalTokens,
			&item.StartedAt, &item.FinishedAt, &item.DurationMs, &item.CreatedAt, &item.UpdatedAt,
			&item.ErrorMessage,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// continueSession reuses the CLI native session ID to initiate the next round of questions.
func (h *TasksHandler) continueSession(c *gin.Context) {
	parentSessionUUID := c.Param("uuid")
	var body struct {
		Content   string `json:"content"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sessionUUID, err := h.orchestrator().ContinueConversation(
		c.Request.Context(),
		workflow.ContinueConversationOptions{
			ParentSessionUUID: parentSessionUUID,
			Prompt:            body.Content,
			RequestID:         body.RequestID,
		},
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	var taskUUID string
	if queryErr := h.taskDB().QueryRowContext(c.Request.Context(), `SELECT task_uuid FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&taskUUID); queryErr == nil {
		_ = markTaskNotificationsRead(c.Request.Context(), h.taskDB(), taskUUID, true)
		h.pushTaskSync(taskUUID)
	}
	c.JSON(http.StatusAccepted, gin.H{"session_uuid": sessionUUID})
}

// stopSession stops the session
func (h *TasksHandler) stopSession(c *gin.Context) {
	sessionUUID := c.Param("uuid")
	err := h.orchestrator().StopSession(c.Request.Context(), sessionUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var taskUUID string
	if queryErr := h.taskDB().QueryRowContext(c.Request.Context(), `SELECT task_uuid FROM gt_cli_sessions WHERE uuid = ?`, sessionUUID).Scan(&taskUUID); queryErr == nil {
		h.pushTaskSync(taskUUID)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// listWorkItems List of work items that can be imported
func (h *TasksHandler) listWorkItems(c *gin.Context) {
	items, err := h.taskCloudClient().GetWorkItems(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []cloud.WorkItem{}
	}

	// The same requirement/defect can only belong to one task: filter out items already occupied by other tasks from the import list
	if sess, ok := getSession(c); ok {
		used := make(map[string]bool)
		rows, qErr := h.taskDB().Query(
			`SELECT work_item_type, work_item_id FROM gt_tasks WHERE source_type = 'cloud' AND cloud_source_key = ? AND cloud_user_id = ?`,
			sess.AdminID, sess.UserID)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var t, id string
				if rows.Scan(&t, &id) == nil {
					used[t+":"+id] = true
				}
			}
		}
		filtered := make([]cloud.WorkItem, 0, len(items))
		for _, it := range items {
			if !used[it.Type+":"+strconv.FormatInt(it.ID, 10)] {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// listPipelines available Pipeline list
func (h *TasksHandler) listPipelines(c *gin.Context) {
	pipelines, err := h.taskCloudClient().GetPipelines(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if pipelines == nil {
		pipelines = []cloud.PipelineSummary{}
	}
	c.JSON(http.StatusOK, gin.H{"items": pipelines})
}

// getPipelineSnapshot Pipeline snapshot
func (h *TasksHandler) getPipelineSnapshot(c *gin.Context) {
	pipelineIDStr := c.Param("id")
	pipelineID, err := strconv.ParseInt(pipelineIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的流水线 ID"})
		return
	}

	snapshot, err := h.taskCloudClient().GetPipelineSnapshot(c.Request.Context(), pipelineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func normalizeExistingDirectory(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("工作目录不能为空")
	}

	absolutePath, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("工作目录地址无效: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)

	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("工作目录不存在: %s", absolutePath)
		}
		return "", fmt.Errorf("无法访问工作目录 %s: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("工作目录地址不是文件夹: %s", absolutePath)
	}

	return absolutePath, nil
}

func normalizeExistingDirectories(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("至少需要一个工作目录")
	}

	workDirs := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for index, value := range raw {
		workDir, err := normalizeExistingDirectory(value)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个工作目录无效: %w", index+1, err)
		}
		key := workDir
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("工作目录不能重复: %s", workDir)
		}
		seen[key] = struct{}{}
		workDirs = append(workDirs, workDir)
	}
	return workDirs, nil
}

// ====== Task status (kanban column) configuration ======

type taskLaneRow struct {
	ID        int64  `json:"id"`
	LaneKey   string `json:"lane_key"`
	Title     string `json:"title"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	IsHidden  int    `json:"is_hidden"`
	TaskCount int    `json:"task_count"`
}

func (h *TasksHandler) listTaskLanes(c *gin.Context) {
	rows, err := h.taskDB().Query(`
		SELECT l.id, l.lane_key, l.title, l.color, l.sort_order, l.is_hidden,
		       (SELECT COUNT(*) FROM gt_tasks t WHERE t.status = l.lane_key) AS task_count
		FROM gt_task_lanes l
		ORDER BY l.sort_order`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var lanes []taskLaneRow
	for rows.Next() {
		var l taskLaneRow
		rows.Scan(&l.ID, &l.LaneKey, &l.Title, &l.Color, &l.SortOrder, &l.IsHidden, &l.TaskCount)
		lanes = append(lanes, l)
	}
	if lanes == nil {
		lanes = []taskLaneRow{}
	}
	c.JSON(http.StatusOK, gin.H{"items": lanes})
}

func (h *TasksHandler) createTaskLane(c *gin.Context) {
	var req struct {
		Title string `json:"title" binding:"required"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title 不能为空"})
		return
	}
	if req.Color == "" {
		req.Color = "#8c8c8c"
	}

	db := h.taskDB()
	now := time.Now().UnixMilli()

	// Generate lane_key: based on title pinyin or random
	laneKey := fmt.Sprintf("lane_%d", now)

	// Get the current maximum sort_order
	var maxOrder int
	db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) FROM gt_task_lanes`).Scan(&maxOrder)

	result, err := db.Exec(
		`INSERT INTO gt_task_lanes (lane_key, title, color, sort_order, is_hidden, created_at) VALUES (?, ?, ?, ?, 0, ?)`,
		laneKey, strings.TrimSpace(req.Title), req.Color, maxOrder+1, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取新建泳道 ID 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "lane_key": laneKey})
}

func (h *TasksHandler) updateTaskLane(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Title    *string `json:"title"`
		Color    *string `json:"color"`
		IsHidden *int    `json:"is_hidden"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sets := []string{}
	args := []interface{}{}
	if req.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, strings.TrimSpace(*req.Title))
	}
	if req.Color != nil {
		sets = append(sets, "color = ?")
		args = append(args, *req.Color)
	}
	if req.IsHidden != nil {
		sets = append(sets, "is_hidden = ?")
		args = append(args, *req.IsHidden)
	}
	if len(sets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无更新字段"})
		return
	}

	args = append(args, id)
	_, err = h.taskDB().Exec(`UPDATE gt_task_lanes SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *TasksHandler) deleteTaskLane(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	db := h.taskDB()

	// Check if there are any tasks using this status
	var laneKey string
	err = db.QueryRow(`SELECT lane_key FROM gt_task_lanes WHERE id = ?`, id).Scan(&laneKey)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "状态不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM gt_tasks WHERE status = ?`, laneKey).Scan(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("该状态下还有 %d 个任务，无法删除", count)})
		return
	}

	_, err = db.Exec(`DELETE FROM gt_task_lanes WHERE id = ?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *TasksHandler) reorderTaskLanes(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids 不能为空"})
		return
	}

	db := h.taskDB()
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	for i, id := range req.IDs {
		if _, err := tx.Exec(`UPDATE gt_task_lanes SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
