package apimanager

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/applog"
)

// Folder is the interface directory within the collection.
type Folder struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collection_id"`
	ParentID     int64  `json:"parent_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	SortOrder    int    `json:"sort_order"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

func (h *Handler) ListFolders(c *gin.Context) {
	collectionID, err := strconv.ParseInt(c.Query("collection_id"), 10, 64)
	if err != nil || collectionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id 参数无效"})
		return
	}
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, collection_id, parent_id, name, description, sort_order, created_at, updated_at
		 FROM gt_api_folders WHERE collection_id=? ORDER BY sort_order, id`, collectionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询目录失败: " + err.Error()})
		return
	}
	defer rows.Close()

	list := []Folder{}
	for rows.Next() {
		var item Folder
		if err := rows.Scan(&item.ID, &item.CollectionID, &item.ParentID, &item.Name,
			&item.Description, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描目录失败: " + err.Error()})
			return
		}
		list = append(list, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) CreateFolder(c *gin.Context) {
	var input struct {
		CollectionID int64  `json:"collection_id"`
		ParentID     int64  `json:"parent_id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.CollectionID <= 0 || input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id 和 name 不能为空"})
		return
	}
	now := nowMillis()
	result, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_api_folders
		 (collection_id, parent_id, name, description, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(sort_order) + 1 FROM gt_api_folders WHERE collection_id=? AND parent_id=?), 0), ?, ?)`,
		input.CollectionID, input.ParentID, input.Name, input.Description,
		input.CollectionID, input.ParentID, now, now)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "创建目录失败: " + err.Error()})
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取新建目录 ID 失败: " + err.Error()})
		return
	}
	var sortOrder int
	if err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT sort_order FROM gt_api_folders WHERE id=?`, id).Scan(&sortOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取新目录排序失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, Folder{
		ID: id, CollectionID: input.CollectionID, ParentID: input.ParentID, Name: input.Name,
		Description: input.Description, SortOrder: sortOrder, CreatedAt: now, UpdatedAt: now,
	})
}

// ReorderFolders persists the order of folders in the root directory of the collection.
func (h *Handler) ReorderFolders(c *gin.Context) {
	var input struct {
		CollectionID int64   `json:"collection_id"`
		FolderIDs    []int64 `json:"folder_ids"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.CollectionID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection_id 和 folder_ids 参数无效"})
		return
	}

	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开始排序事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(c.Request.Context(),
		`SELECT id FROM gt_api_folders WHERE collection_id=? AND parent_id=0`, input.CollectionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询目录失败: " + err.Error()})
		return
	}
	existing := make(map[int64]struct{}, len(input.FolderIDs))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描目录失败: " + err.Error()})
			return
		}
		existing[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取目录失败: " + err.Error()})
		return
	}
	rows.Close()

	if len(existing) != len(input.FolderIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folder_ids 必须包含集合内的全部根目录"})
		return
	}
	seen := make(map[int64]struct{}, len(input.FolderIDs))
	for _, id := range input.FolderIDs {
		if _, ok := existing[id]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "folder_ids 包含不属于当前集合的目录"})
			return
		}
		if _, duplicate := seen[id]; duplicate {
			c.JSON(http.StatusBadRequest, gin.H{"error": "folder_ids 不能包含重复目录"})
			return
		}
		seen[id] = struct{}{}
	}

	now := nowMillis()
	for order, id := range input.FolderIDs {
		if _, err := tx.ExecContext(c.Request.Context(),
			`UPDATE gt_api_folders SET sort_order=?, updated_at=? WHERE id=? AND collection_id=? AND parent_id=0`,
			order, now, id, input.CollectionID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新目录排序失败: " + err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交目录排序失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": len(input.FolderIDs)})
}

func (h *Handler) UpdateFolder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}
	result, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_api_folders SET name=?, description=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(input.Name), input.Description, nowMillis(), id)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "更新目录失败: " + err.Error()})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取更新影响行数失败: " + err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "目录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "updated": true})
}

func (h *Handler) DeleteFolder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	// Deleting the directory will not delete the interface by mistake: requests in the directory are moved back to the root of the collection.
	if _, err = tx.ExecContext(c.Request.Context(),
		`UPDATE gt_api_requests SET folder_id=0, updated_at=? WHERE folder_id=?`, nowMillis(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移动目录内接口失败: " + err.Error()})
		return
	}
	result, err := tx.ExecContext(c.Request.Context(), `DELETE FROM gt_api_folders WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除目录失败: " + err.Error()})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取删除影响行数失败: " + err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "目录不存在"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "deleted": true})
}

// RunHistory is the local execution record used by the response area.
type RunHistory struct {
	ID                    int64               `json:"id"`
	RunUUID               string              `json:"run_uuid"`
	APIID                 int64               `json:"api_id"`
	EnvironmentID         int64               `json:"environment_id"`
	Method                string              `json:"method"`
	URL                   string              `json:"url"`
	RequestHeaders        map[string]string   `json:"request_headers"`
	RequestBodyPreview    string              `json:"request_body_preview"`
	ResponseStatus        int                 `json:"response_status"`
	ResponseHeaders       map[string][]string `json:"response_headers"`
	ResponseBody          string              `json:"response_body"`
	ResponseBodyTruncated bool                `json:"response_body_truncated"`
	DurationMS            int64               `json:"duration_ms"`
	ErrorSummary          string              `json:"error_summary"`
	StartedAt             int64               `json:"started_at"`
}

func (h *Handler) ListHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	if !h.requireStoredRequestInScope(c, id) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, run_uuid, api_id, env_id, method, url, request_headers_json, request_body_preview,
		        response_status, response_headers_json, response_body, response_body_truncated,
		        duration_ms, error_summary, started_at
		 FROM gt_api_runs WHERE api_id=? ORDER BY started_at DESC LIMIT ?`, id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询执行历史失败: " + err.Error()})
		return
	}
	defer rows.Close()

	list := []RunHistory{}
	for rows.Next() {
		var item RunHistory
		var requestHeadersJSON, responseHeadersJSON string
		var truncated int
		if err := rows.Scan(&item.ID, &item.RunUUID, &item.APIID, &item.EnvironmentID,
			&item.Method, &item.URL, &requestHeadersJSON, &item.RequestBodyPreview,
			&item.ResponseStatus, &responseHeadersJSON, &item.ResponseBody, &truncated,
			&item.DurationMS, &item.ErrorSummary, &item.StartedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描执行历史失败: " + err.Error()})
			return
		}
		vars, perr := parseStringMap(requestHeadersJSON)
		if perr != nil {
			applog.Warn("[API] 解析请求头失败", "error", perr)
		}
		item.RequestHeaders = vars
		item.ResponseHeaders = map[string][]string{}
		_ = json.Unmarshal([]byte(responseHeadersJSON), &item.ResponseHeaders)
		item.ResponseBodyTruncated = truncated != 0
		list = append(list, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) ClearHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}
	if !h.requireStoredRequestInScope(c, id) {
		return
	}
	result, err := h.dbRef.Get().ExecContext(c.Request.Context(), `DELETE FROM gt_api_runs WHERE api_id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清理执行历史失败: " + err.Error()})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取清理影响行数失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": affected})
}

func (h *Handler) requireStoredRequestInScope(c *gin.Context, requestID int64) bool {
	var collectionID, folderID int64
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT collection_id, folder_id FROM gt_api_requests WHERE id=?`, requestID,
	).Scan(&collectionID, &folderID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	return true
}

// ParseCurl converts common curl commands into editable request drafts without executing the commands directly.
func (h *Handler) ParseCurl(c *gin.Context) {
	var input struct {
		Curl string `json:"curl"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Curl) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "curl 不能为空"})
		return
	}
	draft, err := parseCurlCommand(input.Curl)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Curl 解析失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, draft)
}

func parseCurlCommand(command string) (APIRequest, error) {
	command = strings.ReplaceAll(command, "\\\r\n", " ")
	command = strings.ReplaceAll(command, "\\\n", " ")
	command = strings.ReplaceAll(command, "^\r\n", " ")
	command = strings.ReplaceAll(command, "^\n", " ")
	tokens := tokenizeCommand(command)
	draft := APIRequest{
		Name: "Curl 导入接口", Method: http.MethodGet, Headers: map[string]string{},
		Query: []KeyValue{}, Auth: AuthConfig{Type: "none"}, BodyType: "none", BodyForm: []KeyValue{},
	}
	var rawURL, data string
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		next := func() string {
			if i+1 >= len(tokens) {
				return ""
			}
			i++
			return tokens[i]
		}
		switch token {
		case "curl", "curl.exe", "--compressed", "-s", "--silent", "-L", "--location":
		case "-X", "--request":
			draft.Method = strings.ToUpper(next())
		case "--url":
			rawURL = next()
		case "-H", "--header":
			header := next()
			if key, value, ok := strings.Cut(header, ":"); ok {
				draft.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		case "-b", "--cookie":
			draft.Headers["Cookie"] = next()
		case "-u", "--user":
			username, password, _ := strings.Cut(next(), ":")
			draft.Auth = AuthConfig{Type: "basic", Username: username, Password: password}
		case "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode":
			data = next()
			if draft.Method == http.MethodGet {
				draft.Method = http.MethodPost
			}
		case "-F", "--form":
			key, value, _ := strings.Cut(next(), "=")
			draft.BodyForm = append(draft.BodyForm, KeyValue{Key: key, Value: value, Enabled: true})
			draft.BodyType = "multipart"
			if draft.Method == http.MethodGet {
				draft.Method = http.MethodPost
			}
		default:
			if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
				rawURL = token
			}
		}
	}
	if rawURL == "" {
		return draft, errors.New("未找到请求 URL")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return draft, err
	}
	for key, values := range parsedURL.Query() {
		for _, value := range values {
			draft.Query = append(draft.Query, KeyValue{Key: key, Value: value, Enabled: true})
		}
	}
	parsedURL.RawQuery = ""
	draft.URL = parsedURL.String()
	draft.Name = parsedURL.Path
	if draft.Name == "" || draft.Name == "/" {
		draft.Name = parsedURL.Host
	}

	contentType := ""
	for key, value := range draft.Headers {
		if strings.EqualFold(key, "Content-Type") {
			contentType = strings.ToLower(value)
			break
		}
		if strings.EqualFold(key, "Authorization") && strings.HasPrefix(strings.ToLower(value), "basic ") {
			raw, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(value[6:]))
			if decodeErr == nil {
				username, password, _ := strings.Cut(string(raw), ":")
				draft.Auth = AuthConfig{Type: "basic", Username: username, Password: password}
				delete(draft.Headers, key)
			}
		}
	}
	if data != "" && draft.BodyType == "none" {
		switch {
		case strings.Contains(contentType, "application/x-www-form-urlencoded"):
			draft.BodyType = "form"
			values, _ := url.ParseQuery(data)
			for key, items := range values {
				for _, value := range items {
					draft.BodyForm = append(draft.BodyForm, KeyValue{Key: key, Value: value, Enabled: true})
				}
			}
		case strings.Contains(contentType, "json") || json.Valid([]byte(data)):
			draft.BodyType = "json"
			draft.Body = data
		default:
			draft.BodyType = "text"
			draft.Body = data
		}
	}
	return draft, nil
}

func tokenizeCommand(value string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, char := range value {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if unicode.IsSpace(char) {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	flush()
	return tokens
}
