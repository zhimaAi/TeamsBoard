package apimanager

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/applog"
	"goteams-client/internal/secrets"
	"goteams-client/internal/skills"
	"goteams-client/internal/storage"
)

// Collection interface collection
type Collection struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    int64  `json:"parent_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// APIRequest interface request
type APIRequest struct {
	ID            int64             `json:"id"`
	CollectionID  int64             `json:"collection_id"`
	FolderID      int64             `json:"folder_id"`
	Name          string            `json:"name"`
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
	Query         []KeyValue        `json:"query"`
	Auth          AuthConfig        `json:"auth"`
	Body          string            `json:"body"`
	BodyType      string            `json:"body_type"`
	BodyForm      []KeyValue        `json:"body_form"`
	EnvironmentID int64             `json:"environment_id"`
	Description   string            `json:"description"`
	CreatedAt     int64             `json:"created_at"`
	UpdatedAt     int64             `json:"updated_at"`
}

// KeyValue is a key value item shared by Params, form and environment editors.
type KeyValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Type    string `json:"type,omitempty"`
}

// AuthConfig describes request-level authentication. Sensitive fields are only used for local execution and are not written to the execution history.
type AuthConfig struct {
	Type      string `json:"type"`
	Token     string `json:"token,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
	AddTo     string `json:"add_to,omitempty"`
	HasSecret bool   `json:"has_secret,omitempty"`
}

// Environment environment
type Environment struct {
	ID           int64             `json:"id"`
	CollectionID int64             `json:"collection_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Variables    map[string]string `json:"variables"`
	CreatedAt    int64             `json:"created_at"`
	UpdatedAt    int64             `json:"updated_at"`
}

// Handler interface manages HTTP processor
type Handler struct {
	dbRef       *storage.DBRef
	secretStore secrets.Store
	accountIDFn func() (string, error)
}

// NewHandler creates an interface management processor
// accountIDFn is used to isolate keys according to the current account to avoid key serial numbers when different accounts use the same ID in the DB.
// secretStore is an account-specific entrusted storage (switched to the corresponding account directory by bootstrap when logging in).
func NewHandler(dbRef *storage.DBRef, accountIDFn func() (string, error), secretStore secrets.Store) *Handler {
	return &Handler{dbRef: dbRef, secretStore: secretStore, accountIDFn: accountIDFn}
}

// requireSkill middleware to verify goteams-api Skill switch (write operation)
func (h *Handler) requireSkill(c *gin.Context) {
	if err := skills.Require(h.dbRef.Get(), skills.SkillAPI); err != nil {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.Next()
}

// nowMillis returns the current Unix millisecond timestamp
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// ==================== Collection management ====================

// ListCollections lists all collections (supports filtering by parent_id)
// GET /api/local/apis/collections?parent_id=X
func (h *Handler) ListCollections(c *gin.Context) {
	parentIDStr := c.Query("parent_id")

	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, name, description, parent_id, created_at, updated_at FROM gt_api_collections ORDER BY id ASC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询集合失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []Collection
	for rows.Next() {
		var col Collection
		if err := rows.Scan(&col.ID, &col.Name, &col.Description, &col.ParentID, &col.CreatedAt, &col.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描集合数据失败: " + err.Error()})
			return
		}
		list = append(list, col)
	}

	// If parent_id is specified, filter
	if parentIDStr != "" {
		parentID, err := strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent_id 参数无效"})
			return
		}
		filtered := make([]Collection, 0, len(list))
		for _, col := range list {
			if col.ParentID == parentID {
				filtered = append(filtered, col)
			}
		}
		list = filtered
	}
	if grant, scoped := capabilityGrant(c); scoped {
		filtered := make([]Collection, 0, 1)
		for _, col := range list {
			if col.ID == grant.APICollectionID {
				filtered = append(filtered, col)
			}
		}
		list = filtered
	}

	if list == nil {
		list = []Collection{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// CreateCollection creates a collection
// POST /api/local/apis/collections
func (h *Handler) CreateCollection(c *gin.Context) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		ParentID    int64  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_api_collections (name, description, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		input.Name, input.Description, input.ParentID, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建集合失败: " + err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取新建集合 ID 失败: " + err.Error()})
		return
	}

	// When creating a new collection, an empty "default environment" is created to facilitate users to directly configure variables.
	if _, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_api_environments (collection_id, name, description, variables_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, "默认环境", "", "{}", now, now); err != nil {
		applog.Warn("创建集合默认环境失败", "collection_id", id, "error", err.Error())
	}

	col := Collection{
		ID:          id,
		Name:        input.Name,
		Description: input.Description,
		ParentID:    input.ParentID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	c.JSON(http.StatusCreated, col)
}

// UpdateCollection update collection
// PUT /api/local/apis/collections/:id
func (h *Handler) UpdateCollection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}

	// All fields use pointers: when the front-end drags and moves the collection, only {"parent_id": N} will be passed.
	//Using value types will overwrite unsubmitted name / description into empty strings.
	var input struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		ParentID    *int64  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	setClauses := make([]string, 0, 4)
	args := make([]interface{}, 0, 5)
	if input.Name != nil {
		if strings.TrimSpace(*input.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "集合名称不能为空"})
			return
		}
		setClauses = append(setClauses, "name=?")
		args = append(args, strings.TrimSpace(*input.Name))
	}
	if input.Description != nil {
		setClauses = append(setClauses, "description=?")
		args = append(args, *input.Description)
	}
	if input.ParentID != nil {
		if *input.ParentID == id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "不能将集合移动到自身下"})
			return
		}
		setClauses = append(setClauses, "parent_id=?")
		args = append(args, *input.ParentID)
	}
	if len(setClauses) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}

	now := nowMillis()
	setClauses = append(setClauses, "updated_at=?")
	args = append(args, now, id)

	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_api_collections SET `+strings.Join(setClauses, ", ")+` WHERE id=?`, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新集合失败: " + err.Error()})
		return
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "集合不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "updated_at": now})
}

// DeleteCollection deletes the collection (cascade delete subrequest)
// DELETE /api/local/apis/collections/:id
func (h *Handler) DeleteCollection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	var taskCount int
	if err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT COUNT(*) FROM gt_tasks WHERE api_collection_id = ?`, id,
	).Scan(&taskCount); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查集合任务引用失败: " + err.Error()})
		return
	}
	if taskCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该集合已被任务引用，不能删除"})
		return
	}

	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()

	var requestIDs []int64
	requestRows, err := tx.QueryContext(c.Request.Context(),
		`SELECT id FROM gt_api_requests WHERE collection_id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询集合接口失败"})
		return
	}
	for requestRows.Next() {
		var requestID int64
		if requestRows.Scan(&requestID) == nil {
			requestIDs = append(requestIDs, requestID)
		}
	}
	requestRows.Close()

	type environmentSecrets struct {
		id        int64
		variables map[string]string
	}
	var environmentSecretList []environmentSecrets
	environmentRows, err := tx.QueryContext(c.Request.Context(),
		`SELECT id, variables_json FROM gt_api_environments WHERE collection_id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询集合环境失败"})
		return
	}
	for environmentRows.Next() {
		var environmentID int64
		var variablesJSON string
		if environmentRows.Scan(&environmentID, &variablesJSON) == nil {
			vars, perr := parseStringMap(variablesJSON)
			if perr != nil {
				applog.Warn("[API] 解析集合环境变量失败，相关密钥可能残留", "environment", environmentID, "error", perr)
			}
			environmentSecretList = append(environmentSecretList, environmentSecrets{
				id: environmentID, variables: vars,
			})
		}
	}
	environmentRows.Close()

	// Execution history is local dependent data of the interface.
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_api_runs WHERE api_id IN (SELECT id FROM gt_api_requests WHERE collection_id=?)`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除集合执行历史失败: " + err.Error()})
		return
	}

	//Delete all requests under the collection
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_api_requests WHERE collection_id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除集合下请求失败: " + err.Error()})
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_api_folders WHERE collection_id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除集合目录失败: " + err.Error()})
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_api_environments WHERE collection_id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除集合环境失败: " + err.Error()})
		return
	}

	// Delete the collection itself
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_api_collections WHERE id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除集合失败: " + err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}
	if h.secretStore != nil {
		for _, requestID := range requestIDs {
			if key, err := h.authSecretKey(requestID); err == nil {
				if dErr := h.secretStore.Delete(key); dErr != nil {
					applog.Warn("[API] 清除请求密钥失败", "error", dErr)
				}
			}
		}
	}
	for _, environment := range environmentSecretList {
		h.deleteEnvironmentSecrets(environment.id, environment.variables)
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "deleted": true})
}

// ==================== Request management ====================

// ListRequests lists the requests under the collection
// GET /api/local/apis/requests?collection_id=X
func (h *Handler) ListRequests(c *gin.Context) {
	collectionIDStr := c.Query("collection_id")
	if collectionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id 参数不能为空"})
		return
	}
	collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id 参数无效"})
		return
	}

	folderIDStr := c.Query("folder_id")
	query := `SELECT id, collection_id, folder_id, name, method, url, headers_json, query_json, auth_json,
	                 body, body_type, body_form_json, environment_id, description, created_at, updated_at
	          FROM gt_api_requests WHERE collection_id=?`
	args := []any{collectionID}
	if grant, scoped := capabilityGrant(c); scoped {
		if collectionID != grant.APICollectionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "集合不在当前任务授权范围内"})
			return
		}
		query += ` AND folder_id=?`
		args = append(args, grant.APIFolderID)
	} else if folderIDStr != "" {
		folderID, parseErr := strconv.ParseInt(folderIDStr, 10, 64)
		if parseErr != nil || folderID < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "folder_id 参数无效"})
			return
		}
		query += ` AND folder_id=?`
		args = append(args, folderID)
	}
	query += ` ORDER BY id ASC`
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询请求失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []APIRequest
	for rows.Next() {
		var req APIRequest
		var headersJSON, queryJSON, authJSON, bodyFormJSON string
		if err := rows.Scan(&req.ID, &req.CollectionID, &req.FolderID, &req.Name, &req.Method, &req.URL,
			&headersJSON, &queryJSON, &authJSON, &req.Body, &req.BodyType, &bodyFormJSON,
			&req.EnvironmentID, &req.Description, &req.CreatedAt, &req.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描请求数据失败: " + err.Error()})
			return
		}
		decodeRequestJSON(&req, headersJSON, queryJSON, authJSON, bodyFormJSON)
		list = append(list, req)
	}

	if list == nil {
		list = []APIRequest{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// GetRequest gets request details
// GET /api/local/apis/requests/:id
func (h *Handler) GetRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}

	var req APIRequest
	var headersJSON, queryJSON, authJSON, bodyFormJSON string
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, collection_id, folder_id, name, method, url, headers_json, query_json, auth_json,
		        body, body_type, body_form_json, environment_id, description, created_at, updated_at
		 FROM gt_api_requests WHERE id=?`, id).
		Scan(&req.ID, &req.CollectionID, &req.FolderID, &req.Name, &req.Method, &req.URL,
			&headersJSON, &queryJSON, &authJSON, &req.Body, &req.BodyType, &bodyFormJSON,
			&req.EnvironmentID, &req.Description, &req.CreatedAt, &req.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询请求失败: " + err.Error()})
		return
	}

	decodeRequestJSON(&req, headersJSON, queryJSON, authJSON, bodyFormJSON)
	if !requireRequestInCapabilityScope(c, req.CollectionID, req.FolderID) {
		return
	}
	c.JSON(http.StatusOK, req)
}

// CreateRequest Create request
// POST /api/local/apis/requests
func (h *Handler) CreateRequest(c *gin.Context) {
	var input struct {
		CollectionID  int64             `json:"collection_id"`
		FolderID      int64             `json:"folder_id"`
		Name          string            `json:"name"`
		Method        string            `json:"method"`
		URL           string            `json:"url"`
		Headers       map[string]string `json:"headers"`
		Query         []KeyValue        `json:"query"`
		Auth          AuthConfig        `json:"auth"`
		Body          string            `json:"body"`
		BodyType      string            `json:"body_type"`
		BodyForm      []KeyValue        `json:"body_form"`
		EnvironmentID int64             `json:"environment_id"`
		Description   string            `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}
	if !requireRequestInCapabilityScope(c, input.CollectionID, input.FolderID) {
		return
	}
	if input.Method == "" {
		input.Method = "GET"
	}
	if input.Headers == nil {
		input.Headers = map[string]string{}
	}
	input.BodyType = normalizeBodyType(input.BodyType)

	headersJSON, err := jsonStringMap(input.Headers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 headers 失败: " + err.Error()})
		return
	}
	queryJSON, err := json.Marshal(input.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 query 失败: " + err.Error()})
		return
	}
	publicAuth := publicAuthConfig(input.Auth)
	authJSON, err := json.Marshal(publicAuth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 auth 失败: " + err.Error()})
		return
	}
	bodyFormJSON, err := json.Marshal(input.BodyForm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 body_form 失败: " + err.Error()})
		return
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_api_requests
		 (collection_id, folder_id, name, method, url, headers_json, query_json, auth_json, body, body_type,
		  body_form_json, environment_id, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.CollectionID, input.FolderID, input.Name, strings.ToUpper(input.Method), input.URL, headersJSON,
		string(queryJSON), string(authJSON), input.Body, input.BodyType, string(bodyFormJSON),
		input.EnvironmentID, input.Description, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败: " + err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取新建请求 ID 失败: " + err.Error()})
		return
	}
	if err := h.saveAuthSecret(id, input.Auth, false); err != nil {
		_, _ = h.dbRef.Get().ExecContext(c.Request.Context(), `DELETE FROM gt_api_requests WHERE id=?`, id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安全保存认证信息失败"})
		return
	}
	req := APIRequest{
		ID:            id,
		CollectionID:  input.CollectionID,
		FolderID:      input.FolderID,
		Name:          input.Name,
		Method:        strings.ToUpper(input.Method),
		URL:           input.URL,
		Headers:       input.Headers,
		Query:         input.Query,
		Auth:          publicAuthConfig(input.Auth),
		Body:          input.Body,
		BodyType:      input.BodyType,
		BodyForm:      input.BodyForm,
		EnvironmentID: input.EnvironmentID,
		Description:   input.Description,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	c.JSON(http.StatusCreated, req)
}

// UpdateRequest update request
// PUT /api/local/apis/requests/:id
func (h *Handler) UpdateRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}

	var input struct {
		CollectionID  *int64            `json:"collection_id"`
		FolderID      *int64            `json:"folder_id"`
		Name          *string           `json:"name"`
		Method        *string           `json:"method"`
		URL           *string           `json:"url"`
		Headers       map[string]string `json:"headers"`
		Query         *[]KeyValue       `json:"query"`
		Auth          *AuthConfig       `json:"auth"`
		Body          *string           `json:"body"`
		BodyType      *string           `json:"body_type"`
		BodyForm      *[]KeyValue       `json:"body_form"`
		EnvironmentID *int64            `json:"environment_id"`
		Description   *string           `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	// Query existing records first
	var req APIRequest
	var headersJSON, queryJSON, authJSON, bodyFormJSON string
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, collection_id, folder_id, name, method, url, headers_json, query_json, auth_json,
		        body, body_type, body_form_json, environment_id, description, created_at, updated_at
		 FROM gt_api_requests WHERE id=?`, id).
		Scan(&req.ID, &req.CollectionID, &req.FolderID, &req.Name, &req.Method, &req.URL,
			&headersJSON, &queryJSON, &authJSON, &req.Body, &req.BodyType, &bodyFormJSON,
			&req.EnvironmentID, &req.Description, &req.CreatedAt, &req.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询请求失败: " + err.Error()})
		return
	}
	decodeRequestJSON(&req, headersJSON, queryJSON, authJSON, bodyFormJSON)
	if !requireRequestInCapabilityScope(c, req.CollectionID, req.FolderID) {
		return
	}
	if grant, scoped := capabilityGrant(c); scoped {
		if (input.CollectionID != nil && *input.CollectionID != grant.APICollectionID) ||
			(input.FolderID != nil && *input.FolderID != grant.APIFolderID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "不能把接口移动到当前任务授权范围之外"})
			return
		}
	}

	//Apply updates
	if input.CollectionID != nil {
		req.CollectionID = *input.CollectionID
	}
	if input.FolderID != nil {
		req.FolderID = *input.FolderID
	}
	if input.Name != nil {
		req.Name = *input.Name
	}
	if input.Method != nil {
		req.Method = strings.ToUpper(*input.Method)
	}
	if input.URL != nil {
		req.URL = *input.URL
	}
	if input.Body != nil {
		req.Body = *input.Body
	}
	if input.BodyType != nil {
		req.BodyType = normalizeBodyType(*input.BodyType)
	}
	if input.BodyForm != nil {
		req.BodyForm = *input.BodyForm
	}
	if input.Query != nil {
		req.Query = *input.Query
	}
	if input.Auth != nil {
		nextAuth := *input.Auth
		preserveSecret := nextAuth.Type == req.Auth.Type && req.Auth.HasSecret && !authHasSecretValue(nextAuth)
		if err := h.saveAuthSecret(id, nextAuth, preserveSecret); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "安全保存认证信息失败"})
			return
		}
		req.Auth = publicAuthConfig(nextAuth)
		req.Auth.HasSecret = authHasSecretValue(nextAuth) || preserveSecret
	}
	if input.EnvironmentID != nil {
		req.EnvironmentID = *input.EnvironmentID
	}
	if input.Description != nil {
		req.Description = *input.Description
	}
	if input.Headers != nil {
		req.Headers = input.Headers
	}
	if !requireRequestInCapabilityScope(c, req.CollectionID, req.FolderID) {
		return
	}

	newHeadersJSON, err := jsonStringMap(req.Headers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 headers 失败: " + err.Error()})
		return
	}
	newQueryJSON, err := json.Marshal(req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 query 失败: " + err.Error()})
		return
	}
	newAuthJSON, err := json.Marshal(req.Auth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 auth 失败: " + err.Error()})
		return
	}
	newBodyFormJSON, err := json.Marshal(req.BodyForm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 body_form 失败: " + err.Error()})
		return
	}

	now := nowMillis()
	_, err = h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_api_requests
		 SET collection_id=?, folder_id=?, name=?, method=?, url=?, headers_json=?, query_json=?, auth_json=?,
		     body=?, body_type=?, body_form_json=?, environment_id=?, description=?, updated_at=?
		 WHERE id=?`,
		req.CollectionID, req.FolderID, req.Name, req.Method, req.URL, newHeadersJSON,
		string(newQueryJSON), string(newAuthJSON), req.Body, req.BodyType, string(newBodyFormJSON),
		req.EnvironmentID, req.Description, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新请求失败: " + err.Error()})
		return
	}

	req.UpdatedAt = now
	c.JSON(http.StatusOK, req)
}

// DeleteRequest delete request
// DELETE /api/local/apis/requests/:id
func (h *Handler) DeleteRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	var collectionID, folderID int64
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT collection_id, folder_id FROM gt_api_requests WHERE id = ?`, id,
	).Scan(&collectionID, &folderID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询请求失败: " + err.Error()})
		return
	}
	if !requireRequestInCapabilityScope(c, collectionID, folderID) {
		return
	}

	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(c.Request.Context(), `DELETE FROM gt_api_runs WHERE api_id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除执行历史失败: " + err.Error()})
		return
	}
	res, err := tx.ExecContext(c.Request.Context(), `DELETE FROM gt_api_requests WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除请求失败: " + err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}
	if h.secretStore != nil {
		if key, err := h.authSecretKey(id); err == nil {
			if dErr := h.secretStore.Delete(key); dErr != nil {
				applog.Warn("[API] 清除请求密钥失败", "error", dErr)
			}
		}
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取删除影响行数失败: " + err.Error()})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "deleted": true})
}

// ExecuteRequest executes the saved interface request and writes the results to local gt_api_runs.
func (h *Handler) ExecuteRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	var input struct {
		EnvironmentID int64 `json:"environment_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	var request APIRequest
	var headersJSON, queryJSON, authJSON, bodyFormJSON string
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, collection_id, folder_id, name, method, url, headers_json, query_json, auth_json,
		        body, body_type, body_form_json, environment_id, description, created_at, updated_at
		 FROM gt_api_requests WHERE id=?`, id).
		Scan(&request.ID, &request.CollectionID, &request.FolderID, &request.Name, &request.Method, &request.URL,
			&headersJSON, &queryJSON, &authJSON, &request.Body, &request.BodyType, &bodyFormJSON,
			&request.EnvironmentID, &request.Description, &request.CreatedAt, &request.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	decodeRequestJSON(&request, headersJSON, queryJSON, authJSON, bodyFormJSON)
	if !requireRequestInCapabilityScope(c, request.CollectionID, request.FolderID) {
		return
	}
	request.Auth, err = h.loadAuthSecret(request.ID, request.Auth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取请求认证信息失败"})
		return
	}

	variables := map[string]string{}
	effectiveEnvironmentID := input.EnvironmentID
	if effectiveEnvironmentID == 0 {
		effectiveEnvironmentID = request.EnvironmentID
	}
	if effectiveEnvironmentID > 0 {
		var variablesJSON string
		err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
			`SELECT variables_json FROM gt_api_environments WHERE id=?`, effectiveEnvironmentID).Scan(&variablesJSON)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "环境不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		variables, err = h.resolveEnvironmentVariables(effectiveEnvironmentID, variablesJSON)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取环境 Secret 失败"})
			return
		}
	}

	requestURL, err := buildRequestURL(request.URL, request.Query, variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求 URL 无效: " + err.Error()})
		return
	}
	requestBody, bodyReader, contentType, err := buildRequestBody(request, variables)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体无效: " + err.Error()})
		return
	}
	outbound, err := http.NewRequestWithContext(c.Request.Context(), strings.ToUpper(request.Method), requestURL, bodyReader)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "创建请求失败: " + err.Error()})
		return
	}
	for key, value := range request.Headers {
		outbound.Header.Set(key, expandVariables(value, variables))
	}
	if contentType != "" && outbound.Header.Get("Content-Type") == "" {
		outbound.Header.Set("Content-Type", contentType)
	}
	applyAuth(outbound, request.Auth, variables)

	startedAt := nowMillis()
	start := time.Now()
	response, requestErr := (&http.Client{Timeout: 30 * time.Second}).Do(outbound)
	durationMs := time.Since(start).Milliseconds()
	finishedAt := nowMillis()

	statusCode := 0
	responseBody := ""
	responseHeaders := map[string][]string{}
	errorSummary := ""
	truncated := false
	if requestErr != nil {
		errorSummary = requestErr.Error()
	} else {
		defer response.Body.Close()
		statusCode = response.StatusCode
		responseHeaders = response.Header
		const maxResponseBytes = 2 << 20
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		if readErr != nil {
			errorSummary = readErr.Error()
		}
		if len(data) > maxResponseBytes {
			data = data[:maxResponseBytes]
			truncated = true
		}
		responseBody = string(data)
	}

	safeRequestHeaders, _ := json.Marshal(redactHeaders(request.Headers))
	safeResponseHeaders, _ := json.Marshal(redactResponseHeaders(responseHeaders))
	safeRequestURL := redactExpanded(redactURL(requestURL), variables)
	safeRequestBody := redactExpanded(requestBody, variables)
	safeResponseBody := redactJSONSecrets(redactExpanded(responseBody, variables))
	_, _ = h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_api_runs
		 (run_uuid, api_id, collection_id, env_id, method, url, request_headers_json, request_body_preview,
		  response_status, response_headers_json, response_body, response_body_truncated, duration_ms,
		  error_summary, started_at, finished_at, created_at)
		 VALUES (lower(hex(randomblob(16))), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, request.CollectionID, effectiveEnvironmentID, request.Method, safeRequestURL,
		string(safeRequestHeaders), truncateText(safeRequestBody, 4096), statusCode, string(safeResponseHeaders),
		safeResponseBody, boolToInt(truncated), durationMs, errorSummary, startedAt, finishedAt, finishedAt)

	if requestErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "请求失败: " + errorSummary})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":         statusCode,
		"headers":        responseHeaders,
		"body":           responseBody,
		"duration_ms":    durationMs,
		"body_truncated": truncated,
	})
}

// ==================== Environmental Management ====================

// ListEnvironments lists all environments
// GET /api/local/apis/environments
func (h *Handler) ListEnvironments(c *gin.Context) {
	collectionID, _ := strconv.ParseInt(c.Query("collection_id"), 10, 64)
	if grant, scoped := capabilityGrant(c); scoped {
		if collectionID > 0 && collectionID != grant.APICollectionID {
			c.JSON(http.StatusForbidden, gin.H{"error": "集合不在当前任务授权范围内"})
			return
		}
		collectionID = grant.APICollectionID
	}
	query := `SELECT id, collection_id, name, description, variables_json, created_at, updated_at
	          FROM gt_api_environments`
	args := []any{}
	if collectionID > 0 {
		query += ` WHERE collection_id=? OR collection_id=0`
		args = append(args, collectionID)
	}
	query += ` ORDER BY id ASC`
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询环境失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []Environment
	for rows.Next() {
		var env Environment
		var variablesJSON string
		if err := rows.Scan(&env.ID, &env.CollectionID, &env.Name, &env.Description,
			&variablesJSON, &env.CreatedAt, &env.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描环境数据失败: " + err.Error()})
			return
		}
		vars, perr := parseStringMap(variablesJSON)
		if perr != nil {
			applog.Warn("[API] 解析环境变量失败", "environment", env.ID, "error", perr)
		}
		env.Variables = publicEnvironmentVariables(vars)
		list = append(list, env)
	}

	if list == nil {
		list = []Environment{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// CreateEnvironment creates environment
// POST /api/local/apis/environments
func (h *Handler) CreateEnvironment(c *gin.Context) {
	var input struct {
		CollectionID int64             `json:"collection_id"`
		Name         string            `json:"name"`
		Description  string            `json:"description"`
		Variables    map[string]string `json:"variables"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}
	if input.Variables == nil {
		input.Variables = map[string]string{}
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_api_environments
		 (collection_id, name, description, variables_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		input.CollectionID, input.Name, input.Description, "{}", now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建环境失败: " + err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取新建环境 ID 失败: " + err.Error()})
		return
	}
	storedVariables, err := h.saveEnvironmentVariables(id, input.Variables, nil)
	if err != nil {
		_, _ = h.dbRef.Get().ExecContext(c.Request.Context(), `DELETE FROM gt_api_environments WHERE id=?`, id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安全保存环境变量失败"})
		return
	}
	variablesJSON, _ := jsonStringMap(storedVariables)
	if _, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_api_environments SET variables_json=? WHERE id=?`, variablesJSON, id); err != nil {
		_, _ = h.dbRef.Get().ExecContext(c.Request.Context(), `DELETE FROM gt_api_environments WHERE id=?`, id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存环境变量失败"})
		return
	}
	env := Environment{
		ID:           id,
		CollectionID: input.CollectionID,
		Name:         input.Name,
		Description:  input.Description,
		Variables:    publicEnvironmentVariables(storedVariables),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	c.JSON(http.StatusCreated, env)
}

// UpdateEnvironment updates the environment
// PUT /api/local/apis/environments/:id
func (h *Handler) UpdateEnvironment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}

	var input struct {
		CollectionID *int64            `json:"collection_id"`
		Name         *string           `json:"name"`
		Description  *string           `json:"description"`
		Variables    map[string]string `json:"variables"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	// Query existing records first
	var env Environment
	var variablesJSON string
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, collection_id, name, description, variables_json, created_at, updated_at
		 FROM gt_api_environments WHERE id=?`, id).
		Scan(&env.ID, &env.CollectionID, &env.Name, &env.Description,
			&variablesJSON, &env.CreatedAt, &env.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "环境不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询环境失败: " + err.Error()})
		return
	}

	if input.Name != nil {
		env.Name = *input.Name
	}
	if input.CollectionID != nil {
		env.CollectionID = *input.CollectionID
	}
	if input.Description != nil {
		env.Description = *input.Description
	}
	if input.Variables != nil {
		vars, perr := parseStringMap(variablesJSON)
		if perr != nil {
			applog.Warn("[API] 解析环境变量失败", "environment", id, "error", perr)
		}
		env.Variables, err = h.saveEnvironmentVariables(id, input.Variables, vars)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "安全保存环境变量失败"})
			return
		}
	} else {
		vars, perr := parseStringMap(variablesJSON)
		if perr != nil {
			applog.Warn("[API] 解析环境变量失败", "environment", id, "error", perr)
		}
		env.Variables = vars
	}

	newVariablesJSON, err := jsonStringMap(env.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化 variables 失败: " + err.Error()})
		return
	}

	now := nowMillis()
	_, err = h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_api_environments
		 SET collection_id=?, name=?, description=?, variables_json=?, updated_at=? WHERE id=?`,
		env.CollectionID, env.Name, env.Description, newVariablesJSON, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新环境失败: " + err.Error()})
		return
	}

	env.UpdatedAt = now
	env.Variables = publicEnvironmentVariables(env.Variables)
	c.JSON(http.StatusOK, env)
}

// DeleteEnvironment deletes the environment
// DELETE /api/local/apis/environments/:id
func (h *Handler) DeleteEnvironment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}

	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()
	var variablesJSON string
	if err := tx.QueryRowContext(c.Request.Context(),
		`SELECT variables_json FROM gt_api_environments WHERE id=?`, id).Scan(&variablesJSON); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "环境不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询环境失败"})
		}
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(),
		`UPDATE gt_api_requests SET environment_id=0, updated_at=? WHERE environment_id=?`, nowMillis(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解除接口环境引用失败: " + err.Error()})
		return
	}
	res, err := tx.ExecContext(c.Request.Context(), `DELETE FROM gt_api_environments WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除环境失败: " + err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}
	variables, perr := parseStringMap(variablesJSON)
	if perr != nil {
		applog.Warn("[API] 删除环境时解析环境变量失败，相关密钥可能残留", "environment", id, "error", perr)
	} else {
		h.deleteEnvironmentSecrets(id, variables)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取删除影响行数失败: " + err.Error()})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "环境不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "deleted": true})
}

// ==================== Auxiliary functions ====================

// parseStringMap parses a JSON string into map[string]string.
// Return error so that the caller can perceive the damaged JSON (for example, when deleting the environment, it can be used to determine whether the key has residual risks).
func parseStringMap(s string) (map[string]string, error) {
	m := map[string]string{}
	if s == "" {
		return m, nil
	}
	err := json.Unmarshal([]byte(s), &m)
	if err != nil {
		return m, err
	}
	return m, nil
}

// jsonStringMap serializes map[string]string into JSON string
func jsonStringMap(m map[string]string) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func expandVariables(value string, variables map[string]string) string {
	result := value
	for key, variable := range variables {
		result = strings.ReplaceAll(result, "{{"+key+"}}", variable)
		result = strings.ReplaceAll(result, "{"+key+"}", variable)
	}
	return result
}

func redactHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "cookie" || strings.Contains(lower, "token") || strings.Contains(lower, "key") {
			result[key] = "***"
		} else {
			result[key] = value
		}
	}
	return result
}

func redactResponseHeaders(headers map[string][]string) map[string][]string {
	result := make(map[string][]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "set-cookie" || lower == "authorization" || strings.Contains(lower, "token") ||
			strings.Contains(lower, "key") {
			result[key] = []string{"***"}
		} else {
			result[key] = values
		}
	}
	return result
}

func redactExpanded(value string, variables map[string]string) string {
	result := value
	for _, variable := range variables {
		if variable != "" {
			result = strings.ReplaceAll(result, variable, "***")
		}
	}
	return result
}

func redactJSONSecrets(value string) string {
	var payload any
	if json.Unmarshal([]byte(value), &payload) != nil {
		return value
	}
	var redact func(any) any
	redact = func(item any) any {
		switch typed := item.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
					strings.Contains(lower, "password") || strings.Contains(lower, "authorization") ||
					strings.Contains(lower, "cookie") || strings.Contains(lower, "api_key") {
					typed[key] = "***"
				} else {
					typed[key] = redact(child)
				}
			}
		case []any:
			for index, child := range typed {
				typed[index] = redact(child)
			}
		}
		return item
	}
	data, err := json.Marshal(redact(payload))
	if err != nil {
		return value
	}
	return string(data)
}

func truncateText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func decodeRequestJSON(request *APIRequest, headersJSON, queryJSON, authJSON, bodyFormJSON string) {
	vars, perr := parseStringMap(headersJSON)
	if perr != nil {
		applog.Warn("[API] 解析请求头失败", "error", perr)
	}
	request.Headers = vars
	request.Query = []KeyValue{}
	request.BodyForm = []KeyValue{}
	request.Auth = AuthConfig{Type: "none"}
	_ = json.Unmarshal([]byte(queryJSON), &request.Query)
	_ = json.Unmarshal([]byte(authJSON), &request.Auth)
	_ = json.Unmarshal([]byte(bodyFormJSON), &request.BodyForm)
	request.BodyType = normalizeBodyType(request.BodyType)
}

func normalizeBodyType(value string) string {
	switch value {
	case "json", "text", "form", "multipart":
		return value
	default:
		return "none"
	}
}

func buildRequestURL(template string, params []KeyValue, variables map[string]string) (string, error) {
	expanded := expandVariables(template, variables)
	parsed, err := url.Parse(expanded)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for _, item := range params {
		if !item.Enabled || strings.TrimSpace(item.Key) == "" {
			continue
		}
		query.Add(expandVariables(item.Key, variables), expandVariables(item.Value, variables))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func buildRequestBody(request APIRequest, variables map[string]string) (string, io.Reader, string, error) {
	switch request.BodyType {
	case "json":
		body := expandVariables(request.Body, variables)
		if strings.TrimSpace(body) != "" && !json.Valid([]byte(body)) {
			return "", nil, "", errors.New("JSON 格式不正确")
		}
		return body, bytes.NewBufferString(body), "application/json", nil
	case "text":
		body := expandVariables(request.Body, variables)
		return body, bytes.NewBufferString(body), "text/plain; charset=utf-8", nil
	case "form":
		values := url.Values{}
		for _, item := range request.BodyForm {
			if item.Enabled && strings.TrimSpace(item.Key) != "" {
				values.Add(expandVariables(item.Key, variables), expandVariables(item.Value, variables))
			}
		}
		body := values.Encode()
		return body, strings.NewReader(body), "application/x-www-form-urlencoded", nil
	case "multipart":
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		for _, item := range request.BodyForm {
			if item.Enabled && strings.TrimSpace(item.Key) != "" {
				if err := writer.WriteField(expandVariables(item.Key, variables), expandVariables(item.Value, variables)); err != nil {
					return "", nil, "", err
				}
			}
		}
		if err := writer.Close(); err != nil {
			return "", nil, "", err
		}
		return buffer.String(), bytes.NewReader(buffer.Bytes()), writer.FormDataContentType(), nil
	default:
		return "", nil, "", nil
	}
}

func applyAuth(request *http.Request, auth AuthConfig, variables map[string]string) {
	switch auth.Type {
	case "bearer":
		request.Header.Set("Authorization", "Bearer "+expandVariables(auth.Token, variables))
	case "basic":
		request.SetBasicAuth(expandVariables(auth.Username, variables), expandVariables(auth.Password, variables))
	case "api_key":
		key := expandVariables(auth.Key, variables)
		value := expandVariables(auth.Value, variables)
		if auth.AddTo == "query" {
			query := request.URL.Query()
			query.Set(key, value)
			request.URL.RawQuery = query.Encode()
		} else if key != "" {
			request.Header.Set(key, value)
		}
	}
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			query.Set(key, "***")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type authSecret struct {
	Token    string `json:"token,omitempty"`
	Password string `json:"password,omitempty"`
	Value    string `json:"value,omitempty"`
}

func (h *Handler) authSecretKey(id int64) (string, error) {
	accountID := ""
	if h.accountIDFn != nil {
		if aid, err := h.accountIDFn(); err == nil {
			accountID = aid
		}
	}
	return fmt.Sprintf("api-auth-%s-%d", accountID, id), nil
}

func authHasSecretValue(auth AuthConfig) bool {
	return auth.Token != "" || auth.Password != "" || auth.Value != ""
}

func publicAuthConfig(auth AuthConfig) AuthConfig {
	return AuthConfig{
		Type:      auth.Type,
		Username:  auth.Username,
		Key:       auth.Key,
		AddTo:     auth.AddTo,
		HasSecret: authHasSecretValue(auth) || auth.HasSecret,
	}
}

func (h *Handler) saveAuthSecret(id int64, auth AuthConfig, preserve bool) error {
	if preserve {
		return nil
	}
	if h.secretStore == nil {
		if authHasSecretValue(auth) {
			return errors.New("SecretStore 不可用")
		}
		return nil
	}
	if auth.Type == "none" || !authHasSecretValue(auth) {
		if key, err := h.authSecretKey(id); err == nil {
			return h.secretStore.Delete(key)
		}
		return nil
	}
	data, err := json.Marshal(authSecret{
		Token: auth.Token, Password: auth.Password, Value: auth.Value,
	})
	if err != nil {
		return err
	}
	key, err := h.authSecretKey(id)
	if err != nil {
		return err
	}
	return h.secretStore.Set(key, string(data))
}

func (h *Handler) loadAuthSecret(id int64, auth AuthConfig) (AuthConfig, error) {
	if !auth.HasSecret {
		return auth, nil
	}
	if h.secretStore == nil {
		return auth, errors.New("SecretStore 不可用")
	}
	key, err := h.authSecretKey(id)
	if err != nil {
		return auth, err
	}
	value, err := h.secretStore.Get(key)
	if err != nil {
		return auth, err
	}
	var secret authSecret
	if err := json.Unmarshal([]byte(value), &secret); err != nil {
		return auth, err
	}
	auth.Token = secret.Token
	auth.Password = secret.Password
	auth.Value = secret.Value
	return auth, nil
}

const (
	environmentSecretSentinel = "__GOTEAMS_SECRET__"
	environmentSecretMask     = "••••••"
)

func isSensitiveVariableKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") || strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "cookie") || strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "apikey")
}

func (h *Handler) environmentSecretKey(environmentID int64, key string) (string, error) {
	accountID := ""
	if h.accountIDFn != nil {
		if aid, err := h.accountIDFn(); err == nil {
			accountID = aid
		}
	}
	return fmt.Sprintf("api-env-%s-%d-%s", accountID, environmentID, key), nil
}

func publicEnvironmentVariables(variables map[string]string) map[string]string {
	result := make(map[string]string, len(variables))
	for key, value := range variables {
		if value == environmentSecretSentinel {
			result[key] = environmentSecretMask
		} else {
			result[key] = value
		}
	}
	return result
}

func (h *Handler) saveEnvironmentVariables(
	environmentID int64,
	input map[string]string,
	current map[string]string,
) (map[string]string, error) {
	result := make(map[string]string, len(input))
	for key, value := range input {
		if !isSensitiveVariableKey(key) {
			result[key] = value
			continue
		}
		if h.secretStore == nil {
			return nil, errors.New("SecretStore 不可用")
		}
		if (value == "" || value == environmentSecretMask) && current[key] == environmentSecretSentinel {
			result[key] = environmentSecretSentinel
			continue
		}
		if value == "" || value == environmentSecretMask {
			return nil, fmt.Errorf("敏感变量 %s 缺少值", key)
		}
		k, kerr := h.environmentSecretKey(environmentID, key)
		if kerr != nil {
			return nil, kerr
		}
		if err := h.secretStore.Set(k, value); err != nil {
			return nil, err
		}
		result[key] = environmentSecretSentinel
	}
	for key, value := range current {
		if value == environmentSecretSentinel {
			if _, exists := result[key]; !exists && h.secretStore != nil {
				if k, kerr := h.environmentSecretKey(environmentID, key); kerr == nil {
					_ = h.secretStore.Delete(k)
				}
			}
		}
	}
	return result, nil
}

func (h *Handler) resolveEnvironmentVariables(environmentID int64, rawJSON string) (map[string]string, error) {
	result, perr := parseStringMap(rawJSON)
	if perr != nil {
		return nil, perr
	}
	for key, value := range result {
		if value != environmentSecretSentinel {
			continue
		}
		if h.secretStore == nil {
			return nil, errors.New("SecretStore 不可用")
		}
		k, kerr := h.environmentSecretKey(environmentID, key)
		if kerr != nil {
			return nil, kerr
		}
		secret, err := h.secretStore.Get(k)
		if err != nil {
			return nil, err
		}
		result[key] = secret
	}
	return result, nil
}

func (h *Handler) deleteEnvironmentSecrets(environmentID int64, variables map[string]string) {
	if h.secretStore == nil {
		return
	}
	for key, value := range variables {
		if value == environmentSecretSentinel {
			if k, kerr := h.environmentSecretKey(environmentID, key); kerr == nil {
				if dErr := h.secretStore.Delete(k); dErr != nil {
					applog.Warn("[API] 清除环境变量密钥失败", "error", dErr)
				}
			}
		}
	}
}
