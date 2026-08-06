package localserver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/cloud"
	"goteams-client/internal/executor"
	"goteams-client/internal/storage"
	"goteams-client/internal/workflow"
	"goteams-client/internal/workitem"
)

// TasksHandler task-related HTTP handler
//
// Task data (gt_tasks, etc.) is located in the current cloud account session database, and the local Profile library (base.db)
// Independent of each other. sessionDB returns the session database; dbRef points to the local Profile library and is only used for cross-table association
// Local configuration (database Profile, knowledge base document).
type TasksHandler struct {
	dbRef        *storage.DBRef
	sessionDB    func() *sql.DB
	orchestrator func() *workflow.Orchestrator
	workitemSvc  func() *workitem.Service
	cloudClient  func() *cloud.Client
}

// NewTasksHandler creates task handler
func NewTasksHandler(dbRef *storage.DBRef, sessionDB func() *sql.DB, orchestrator func() *workflow.Orchestrator, workitemSvc func() *workitem.Service, cloudClient func() *cloud.Client) *TasksHandler {
	return &TasksHandler{
		dbRef:        dbRef,
		sessionDB:    sessionDB,
		orchestrator: orchestrator,
		workitemSvc:  workitemSvc,
		cloudClient:  cloudClient,
	}
}

// taskDB returns the current account session database (the database where the task data is located).
func (h *TasksHandler) taskDB() *sql.DB {
	if h.sessionDB == nil {
		return nil
	}
	return h.sessionDB()
}

// taskCloudClient returns the cloud client used by the current session (login target: official or custom address).
// When logging in with a custom address, the session holds an independent client; if the session/client is missing, it falls back to the default client held by the processor.
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
	r.POST("/:uuid/steps/:key/runs", h.runStep)
	r.GET("/:uuid/sessions", h.listTaskSessions)
	r.POST("/sessions/:uuid/messages", h.continueSession)
	r.POST("/sessions/:uuid/stop", h.stopSession)
	r.DELETE("/:uuid", h.deleteTask)

	// Work items and Agents
	r.GET("/my-work", h.listMyWork)
	r.GET("/work-items", h.listWorkItems)
	r.GET("/agents", h.listAgents)
	r.GET("/agents/:id/snapshot", h.getAgentSnapshot)
	r.GET("/cli-count", h.getCliTaskCount)
	r.GET("/cli-discovery", h.cliDiscovery)

	// Status (kanban column) configuration
	r.GET("/task-lanes", h.listTaskLanes)
	r.POST("/task-lanes", h.createTaskLane)
	r.PUT("/task-lanes/reorder", h.reorderTaskLanes)
	r.PUT("/task-lanes/:id", h.updateTaskLane)
	r.DELETE("/task-lanes/:id", h.deleteTaskLane)
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
	rows, err := h.taskDB().Query(
		`SELECT work_item_type, work_item_id, uuid FROM gt_tasks WHERE admin_id = ? AND user_id = ?`,
		session.AdminID, session.UserID,
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

// cliDiscovery detects the installed CLI tools and their model configurations on this machine
func (h *TasksHandler) cliDiscovery(c *gin.Context) {
	items := executor.DiscoverCLIs(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// listTasks task list
func (h *TasksHandler) listTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT t.uuid, t.title, t.status, t.execution_status, t.agent_id, t.work_item_type, t.work_item_id, t.created_at, t.updated_at, CASE WHEN t.content_snapshot != '' THEN t.content_snapshot ELSE COALESCE(json_extract(s.snapshot_json, '$.description'), '') END, t.agent_name_snapshot FROM gt_tasks t LEFT JOIN gt_work_item_snapshots s ON s.admin_id = t.admin_id AND s.user_id = t.user_id AND s.work_item_type = t.work_item_type AND s.work_item_id = t.work_item_id`
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
		UUID              string `json:"uuid"`
		Title             string `json:"title"`
		Status            string `json:"status"`
		ExecutionStatus   string `json:"execution_status"`
		AgentID           string `json:"agent_id"`
		WorkItemType      string `json:"work_item_type"`
		WorkItemID        string `json:"work_item_id"`
		CreatedAt         int64  `json:"created_at"`
		UpdatedAt         int64  `json:"updated_at"`
		ContentSnapshot   string `json:"content_snapshot"`
		AgentNameSnapshot string `json:"agent_name_snapshot"`
	}

	items := make([]TaskItem, 0)
	for rows.Next() {
		var t TaskItem
		// Scan must be aborted if it fails: ignoring the error will append the zero-valued TaskItem to the result set.
		//The front end gets the "ghost task" with an empty UUID.
		if err := rows.Scan(&t.UUID, &t.Title, &t.Status, &t.ExecutionStatus, &t.AgentID, &t.WorkItemType, &t.WorkItemID, &t.CreatedAt, &t.UpdatedAt, &t.ContentSnapshot, &t.AgentNameSnapshot); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取任务列表失败: " + err.Error()})
			return
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "遍历任务列表失败: " + err.Error()})
		return
	}

	var total int64
	countQuery := `SELECT COUNT(*) FROM gt_tasks`
	if status != "" {
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
	taskUUID := c.Param("uuid")

	var task map[string]interface{}
	var uuid, title, status, execStatus, agentID, cliType, workItemType, workItemID, workDir string
	var apiCollectionName, apiFolderName string
	var apiCollectionID, apiFolderID int64
	var createdAt, updatedAt int64
	err := h.taskDB().QueryRow(
		`SELECT uuid, title, status, execution_status, agent_id, cli_type, work_item_type, work_item_id, work_dir,
		        api_collection_id, api_collection_name_snapshot, api_folder_id, api_folder_name_snapshot,
		        created_at, updated_at
		 FROM gt_tasks WHERE uuid = ?`,
		taskUUID).Scan(
		&uuid, &title, &status, &execStatus, &agentID, &cliType, &workItemType, &workItemID, &workDir,
		&apiCollectionID, &apiCollectionName, &apiFolderID, &apiFolderName,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	task = map[string]interface{}{
		"uuid":                uuid,
		"title":               title,
		"status":              status,
		"execution_status":    execStatus,
		"agent_id":            agentID,
		"cli_type":            cliType,
		"work_item_type":      workItemType,
		"work_item_id":        workItemID,
		"work_dir":            workDir,
		"api_collection_id":   apiCollectionID,
		"api_collection_name": apiCollectionName,
		"api_folder_id":       apiFolderID,
		"api_folder_name":     apiFolderName,
		"created_at":          createdAt,
		"updated_at":          updatedAt,
	}

	workDirs, err := h.loadTaskWorkDirs(taskUUID, workDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	task["work_dirs"] = workDirs

	dbProfiles, err := h.loadTaskDBProfiles(taskUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	task["database_profiles"] = dbProfiles

	// Query steps
	taskDocuments, err := h.loadTaskDocuments(taskUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rows, err := h.taskDB().Query(
		`SELECT uuid, step_key, name, step_order, cli_type, status, execution_status, prompt_snapshot, documents_snapshot_json
		 FROM gt_task_steps WHERE task_uuid = ? ORDER BY step_order`,
		taskUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var steps []map[string]interface{}
	for rows.Next() {
		var sUUID, stepKey, name, cliType, stepStatus, stepExecStatus, prompt, documentsSnapshotJSON string
		var sortOrder int
		if err := rows.Scan(&sUUID, &stepKey, &name, &sortOrder, &cliType, &stepStatus, &stepExecStatus, &prompt, &documentsSnapshotJSON); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取任务步骤失败: " + err.Error()})
			return
		}
		steps = append(steps, map[string]interface{}{
			"uuid":             sUUID,
			"step_key":         stepKey,
			"name":             name,
			"sort_order":       sortOrder,
			"cli_type":         cliType,
			"status":           stepStatus,
			"execution_status": stepExecStatus,
			"prompt_snapshot":  prompt,
			"documents":        matchStepDocuments(documentsSnapshotJSON, prompt, taskDocuments),
		})
	}
	if steps == nil {
		steps = []map[string]interface{}{}
	}

	task["steps"] = steps
	c.JSON(http.StatusOK, task)
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

func (h *TasksHandler) loadTaskDBProfiles(taskUUID string) ([]map[string]interface{}, error) {
	// The database profile configuration is stored in the local Profile library (base.db), and is associated with the Profile library query here.
	rows, err := h.dbRef.Get().Query(
		`SELECT p.id, p.name, p.db_type, p.database_name
		 FROM gt_task_db_profiles t
		 JOIN gt_database_profiles p ON p.id = t.database_profile_id
		 WHERE t.task_uuid = ?
		 ORDER BY t.sort_order, t.database_profile_id`,
		taskUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询任务数据库配置失败: %w", err)
	}
	defer rows.Close()

	profiles := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var name, dbType, databaseName string
		if err := rows.Scan(&id, &name, &dbType, &databaseName); err != nil {
			return nil, fmt.Errorf("读取任务数据库配置失败: %w", err)
		}
		profiles = append(profiles, map[string]interface{}{
			"id":            id,
			"name":          name,
			"db_type":       dbType,
			"database_name": databaseName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务数据库配置失败: %w", err)
	}
	if profiles == nil {
		profiles = []map[string]interface{}{}
	}
	return profiles, nil
}

type taskDocumentDetail struct {
	UUID             string `json:"uuid"`
	ClientDocumentID string `json:"client_document_id"`
	Name             string `json:"name"`
	Content          string `json:"content"`
	UpdatedAt        int64  `json:"updated_at"`
	Path             string `json:"-"`
}

func (h *TasksHandler) loadTaskDocuments(taskUUID string) ([]taskDocumentDetail, error) {
	//Knowledge base document metadata is stored in the local Profile library (base.db), and is associated with the Profile library query here.
	rows, err := h.dbRef.Get().Query(
		`SELECT td.knowledge_document_uuid, td.client_document_id, td.document_name, td.document_path,
		        COALESCE(kd.updated_at, td.created_at)
		 FROM gt_task_documents td
		 LEFT JOIN gt_knowledge_documents kd ON kd.uuid = td.knowledge_document_uuid AND kd.deleted = 0
		 WHERE td.task_uuid = ?
		 ORDER BY td.id`,
		taskUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询任务文档失败: %w", err)
	}
	defer rows.Close()

	documents := make([]taskDocumentDetail, 0)
	for rows.Next() {
		var document taskDocumentDetail
		if err := rows.Scan(
			&document.UUID,
			&document.ClientDocumentID,
			&document.Name,
			&document.Path,
			&document.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("读取任务文档失败: %w", err)
		}
		if content, readErr := os.ReadFile(document.Path); readErr == nil {
			document.Content = string(content)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历任务文档失败: %w", err)
	}
	return documents, nil
}

func matchStepDocuments(snapshotJSON, prompt string, taskDocuments []taskDocumentDetail) []taskDocumentDetail {
	var definitions []cloud.AgentDocument
	if strings.TrimSpace(snapshotJSON) != "" {
		_ = json.Unmarshal([]byte(snapshotJSON), &definitions)
	}

	matched := make([]taskDocumentDetail, 0, len(definitions))
	seen := make(map[string]struct{})
	appendDocument := func(document taskDocumentDetail) {
		key := document.UUID
		if key == "" {
			key = document.ClientDocumentID + "|" + document.Name
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		matched = append(matched, document)
	}

	for _, definition := range definitions {
		identifiers := []string{
			strings.TrimSpace(definition.ClientDocumentID),
			strings.TrimSpace(definition.ID),
			strings.TrimSpace(definition.Key),
		}
		var found *taskDocumentDetail
		for index := range taskDocuments {
			document := &taskDocuments[index]
			for _, identifier := range identifiers {
				if identifier != "" && identifier == document.ClientDocumentID {
					found = document
					break
				}
			}
			if found == nil && definition.Name != "" && definition.Name == document.Name {
				found = document
			}
			if found != nil {
				break
			}
		}
		if found != nil {
			appendDocument(*found)
			continue
		}

		identifier := ""
		for _, value := range identifiers {
			if value != "" {
				identifier = value
				break
			}
		}
		appendDocument(taskDocumentDetail{
			ClientDocumentID: identifier,
			Name:             definition.Name,
			Content:          definition.DefaultContent,
		})
	}

	// Compatible with historical tasks without step document snapshots: identify ownership through the document path parsed into Prompt.
	if len(definitions) == 0 {
		for _, document := range taskDocuments {
			if document.Path != "" && strings.Contains(prompt, document.Path) {
				appendDocument(document)
				continue
			}
			if baseName := filepath.Base(document.Path); baseName != "." && baseName != "" && strings.Contains(prompt, baseName) {
				appendDocument(document)
			}
		}
	}

	return matched
}

// createTask creates a task
func (h *TasksHandler) createTask(c *gin.Context) {
	var body struct {
		WorkItemType       string   `json:"work_item_type"`
		WorkItemID         int64    `json:"work_item_id"`
		AgentID            int64    `json:"agent_id"`
		WorkDir            string   `json:"work_dir"`
		WorkDirs           []string `json:"work_dirs"`
		APICollectionID    int64    `json:"api_collection_id"`
		APIFolderID        int64    `json:"api_folder_id"`
		DatabaseProfileIDs []int64  `json:"database_profile_ids"`
		Status             string   `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Optional: The initial status of the new task (kanban column), 'pending' is used by the service layer by default.
	// The verification logic is consistent with updateTaskStatus: check the gt_task_lanes table first, and then be compatible with the hard-coded status.
	if body.Status != "" {
		var laneCount int
		h.taskDB().QueryRow(`SELECT COUNT(*) FROM gt_task_lanes WHERE lane_key = ?`, body.Status).Scan(&laneCount)
		if laneCount == 0 && !workflow.ValidateTaskStatus(body.Status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务状态"})
			return
		}
	}

	// Get session information
	session, ok := getSession(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	rawWorkDirs := body.WorkDirs
	if len(rawWorkDirs) == 0 && strings.TrimSpace(body.WorkDir) != "" {
		rawWorkDirs = []string{body.WorkDir}
	}
	workDirs, err := normalizeExistingDirectories(rawWorkDirs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Explicit disclosure: workDirs[0] below relies on the invariant "at least one directory".
	// If we only rely on the internal guarantee of normalizeExistingDirectories, its logic will panic directly once it is adjusted.
	if len(workDirs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "至少需要一个工作目录"})
		return
	}

	// Get work item details from the cloud
	workItems, err := h.taskCloudClient().GetWorkItems(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取工作项失败: " + err.Error()})
		return
	}

	var workItem *cloud.WorkItem
	for _, wi := range workItems {
		if wi.Type == body.WorkItemType && wi.ID == body.WorkItemID {
			workItem = &wi
			break
		}
	}
	if workItem == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "工作项不存在"})
		return
	}

	// Get Agent snapshot from cloud
	agentSnapshot, err := h.taskCloudClient().GetAgentSnapshot(c.Request.Context(), body.AgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Agent 快照失败: " + err.Error()})
		return
	}

	//Create task
	taskUUID, err := h.workitemSvc().CreateTask(c.Request.Context(), workitem.CreateTaskOptions{
		AdminID:         session.AdminID,
		UserID:          session.UserID,
		WorkItem:        *workItem,
		AgentSnapshot:   agentSnapshot,
		WorkDir:         workDirs[0],
		WorkDirs:        workDirs,
		APICollectionID: body.APICollectionID,
		APIFolderID:     body.APIFolderID,
		DBProfileIDs:    body.DatabaseProfileIDs,
		Status:          body.Status,
	})
	if err != nil {
		if errors.Is(err, workitem.ErrWorkItemAlreadyInTask) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, workitem.ErrInvalidAPIScope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, workitem.ErrInvalidDBProfile) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建任务失败: " + err.Error()})
		return
	}

	// Task creation completed: trigger a cloud synchronization immediately
	if orch := h.orchestrator(); orch != nil {
		orch.SyncTaskNow(taskUUID)
	}

	c.JSON(http.StatusCreated, gin.H{"uuid": taskUUID})
}

// updateTaskStatus updates task status
func (h *TasksHandler) updateTaskStatus(c *gin.Context) {
	taskUUID := c.Param("uuid")
	var body struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

	//Task status is changed manually: trigger a cloud synchronization
	if orch := h.orchestrator(); orch != nil {
		orch.SyncTaskNow(taskUUID)
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// deleteTask delete task (cascade deletion of sub-table data)
func (h *TasksHandler) deleteTask(c *gin.Context) {
	taskUUID := c.Param("uuid")

	//Refuse to delete tasks with active Sessions to prevent the CLI process from becoming an orphan that cannot be terminated.
	// If the query fails, an error must be reported directly: swallowing the error will make activeCount always 0, and the protection will be in vain.
	// Instead, cascading deletions of running session records are performed - exactly what this check is intended to prevent.
	var activeCount int
	if err := h.taskDB().QueryRow(
		`SELECT COUNT(*) FROM gt_cli_sessions WHERE task_uuid = ? AND status IN ('created', 'running', 'waiting_input', 'stop_requested')`,
		taskUUID).Scan(&activeCount); err != nil {
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
	if _, err := tx.Exec(`DELETE FROM gt_cli_sessions WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务会话失败: " + err.Error()})
		return
	}
	if _, err := tx.Exec(`DELETE FROM gt_task_sync_state WHERE task_uuid = ?`, taskUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务同步状态失败: " + err.Error()})
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

	// After the local deletion is successful, notify the cloud to delete it synchronously (immediately push online, offline will be reissued in the buffer to be deleted)
	if h.orchestrator != nil {
		if orch := h.orchestrator(); orch != nil {
			orch.SyncTaskDelete(taskUUID)
		}
	}

	c.JSON(http.StatusOK, gin.H{"uuid": taskUUID, "deleted": true})
}

// runStep execution step
func (h *TasksHandler) runStep(c *gin.Context) {
	taskUUID := c.Param("uuid")
	stepKey := c.Param("key")

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
		StepKey:   stepKey,
		WorkDir:   workDir,
		RequestID: body.RequestID,
		CLIType:   body.CLIType,
		Model:     body.Model,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session_uuid": sessionUUID})
}

// listTaskSessions returns all local CLI running records under the task, and the complete events can be played back on demand through WebSocket.
func (h *TasksHandler) listTaskSessions(c *gin.Context) {
	taskUUID := c.Param("uuid")
	rows, err := h.taskDB().Query(
		`SELECT s.uuid, s.task_uuid, s.step_uuid, ts.step_key, ts.name,
		        s.conversation_uuid, s.parent_session_uuid, s.run_no, s.status,
		        s.cli_type, s.external_session_id, s.work_dir, s.prompt_snapshot,
		        s.input_tokens, s.output_tokens, s.total_tokens,
		        s.started_at, s.finished_at, s.duration_ms, s.created_at, s.updated_at
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
			`SELECT work_item_type, work_item_id FROM gt_tasks WHERE admin_id = ? AND user_id = ?`,
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

// listAgents available Agent list
func (h *TasksHandler) listAgents(c *gin.Context) {
	agents, err := h.taskCloudClient().GetAgents(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if agents == nil {
		agents = []cloud.AgentSummary{}
	}
	c.JSON(http.StatusOK, gin.H{"items": agents})
}

// getAgentSnapshot Agent snapshot
func (h *TasksHandler) getAgentSnapshot(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Agent ID"})
		return
	}

	snapshot, err := h.taskCloudClient().GetAgentSnapshot(c.Request.Context(), agentID)
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
