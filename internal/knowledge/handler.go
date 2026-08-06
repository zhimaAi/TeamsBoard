package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"goteams-client/internal/storage"
)

// ==================== Model definition ====================

// FolderNode folder tree node
type FolderNode struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	ParentID  int64        `json:"parent_id"`
	CreatedAt int64        `json:"created_at"`
	UpdatedAt int64        `json:"updated_at"`
	Children  []FolderNode `json:"children,omitempty"`
}

// Document document
type Document struct {
	UUID        string   `json:"uuid"`
	FolderID    int64    `json:"folder_id"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	ContentHash string   `json:"content_hash,omitempty"`
	WordCount   int      `json:"word_count,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
	Content     string   `json:"content,omitempty"`
}

// History historical version
type History struct {
	ID          int64  `json:"id"`
	DocUUID     string `json:"doc_uuid"`
	ContentHash string `json:"content_hash"`
	WordCount   int    `json:"word_count"`
	CreatedAt   int64  `json:"created_at"`
}

// SearchResult search result item
type SearchResult struct {
	UUID     string `json:"uuid"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
	FolderID int64  `json:"folder_id"`
}

// ==================== Handler ====================

// Handler knowledge base HTTP handler
type Handler struct {
	dbRef              *storage.DBRef
	contentDirProvider func() (string, error)
	mu                 sync.Mutex
	stores             map[string]*Store
}

// NewHandler creates a knowledge base handler
// contentDirProvider is used to parse the knowledge base content directory according to the current login account (account directory: <accountDataDir>/knowledge)
func NewHandler(dbRef *storage.DBRef, contentDirProvider func() (string, error)) *Handler {
	return &Handler{
		dbRef:              dbRef,
		contentDirProvider: contentDirProvider,
		stores:             make(map[string]*Store),
	}
}

// getStore parses file storage according to the current login account and caches it according to the content directory.
// The knowledge base file belongs to the account directory: <accountDataDir>/knowledge
func (h *Handler) getStore(c *gin.Context) (*Store, error) {
	if h.contentDirProvider == nil {
		return nil, fmt.Errorf("未配置知识库存储")
	}
	contentDir, err := h.contentDirProvider()
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	s, ok := h.stores[contentDir]
	h.mu.Unlock()
	if !ok {
		s, err = NewStore(contentDir)
		if err != nil {
			return nil, err
		}
		h.mu.Lock()
		h.stores[contentDir] = s
		h.mu.Unlock()
	}
	return s, nil
}

// nowMillis returns the current Unix millisecond timestamp
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// ==================== Folder management ====================

// ListFolders lists all folders (tree shape)
// GET /api/local/knowledge/folders
func (h *Handler) ListFolders(c *gin.Context) {
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, name, parent_id, created_at, updated_at FROM gt_knowledge_folders ORDER BY id ASC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文件夹失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var folders []FolderNode
	for rows.Next() {
		var f FolderNode
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描文件夹数据失败: " + err.Error()})
			return
		}
		folders = append(folders, f)
	}

	tree := buildFolderTree(folders, 0)
	if tree == nil {
		tree = []FolderNode{}
	}
	c.JSON(http.StatusOK, gin.H{"data": tree})
}

// CreateFolder creates a folder
// POST /api/local/knowledge/folders
func (h *Handler) CreateFolder(c *gin.Context) {
	var input struct {
		Name     string `json:"name"`
		ParentID int64  `json:"parent_id"`
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
		`INSERT INTO gt_knowledge_folders (name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		input.Name, input.ParentID, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建文件夹失败: " + err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取新建文件夹 ID 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, FolderNode{
		ID:        id,
		Name:      input.Name,
		ParentID:  input.ParentID,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// UpdateFolder update folder
// PUT /api/local/knowledge/folders/:id
func (h *Handler) UpdateFolder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 参数无效"})
		return
	}

	var input struct {
		Name     *string `json:"name"`
		ParentID *int64  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	//Query existing records
	var name string
	var parentID int64
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT name, parent_id FROM gt_knowledge_folders WHERE id=?`, id).Scan(&name, &parentID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件夹不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文件夹失败: " + err.Error()})
		return
	}

	// Prevent folders from being moved under itself or its descendants.
	// Only judging ParentID == id cannot prevent cross-level movements such as A→A's grandchild node.
	// Once a loop is formed, the entire subtree will disappear from the root node query and cannot be deleted anymore.
	if input.ParentID != nil && *input.ParentID != parentID {
		if *input.ParentID == id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "不能将文件夹移动到自身下"})
			return
		}
		if *input.ParentID != 0 {
			descendant, err := h.isDescendantFolder(c.Request.Context(), *input.ParentID, id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "校验文件夹层级失败: " + err.Error()})
				return
			}
			if descendant {
				c.JSON(http.StatusBadRequest, gin.H{"error": "不能将文件夹移动到其子目录下"})
				return
			}
		}
	}

	if input.Name != nil {
		name = *input.Name
	}
	if input.ParentID != nil {
		parentID = *input.ParentID
	}

	now := nowMillis()
	_, err = h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_folders SET name=?, parent_id=?, updated_at=? WHERE id=?`,
		name, parentID, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新文件夹失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "updated_at": now})
}

// isDescendantFolder determines whether candidateID is a descendant of ancestorID (tracing back up the parent_id chain).
// If the existing data contains loops, use the upper limit of the number of steps to exit to avoid an infinite loop.
func (h *Handler) isDescendantFolder(ctx context.Context, candidateID, ancestorID int64) (bool, error) {
	const maxDepth = 256
	current := candidateID
	for step := 0; step < maxDepth; step++ {
		if current == 0 {
			return false, nil
		}
		if current == ancestorID {
			return true, nil
		}
		var parentID int64
		err := h.dbRef.Get().QueryRowContext(ctx,
			`SELECT parent_id FROM gt_knowledge_folders WHERE id=?`, current).Scan(&parentID)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		current = parentID
	}
	return false, fmt.Errorf("文件夹层级超过 %d 层或存在环", maxDepth)
}

// DeleteFolder deletes the folder
// DELETE /api/local/knowledge/folders/:id
func (h *Handler) DeleteFolder(c *gin.Context) {
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

	//Move the documents in this folder to the root directory
	if _, err := tx.ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET folder_id=0 WHERE folder_id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移动文档失败: " + err.Error()})
		return
	}

	// Move subfolders to the root directory
	if _, err := tx.ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_folders SET parent_id=0 WHERE parent_id=?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移动子文件夹失败: " + err.Error()})
		return
	}

	// Delete the folder itself
	res, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_knowledge_folders WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文件夹失败: " + err.Error()})
		return
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取删除影响行数失败: " + err.Error()})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件夹不存在"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交事务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "deleted": true})
}

// ==================== Document Management ====================

// ListDocuments document list
// GET /api/local/knowledge/documents?folder_id=X&keyword=Y
func (h *Handler) ListDocuments(c *gin.Context) {
	folderIDStr := c.Query("folder_id")
	keyword := c.Query("keyword")

	query := `SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at
		FROM gt_knowledge_documents WHERE deleted=0`
	var args []interface{}

	if folderIDStr != "" {
		folderID, err := strconv.ParseInt(folderIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "folder_id 参数无效"})
			return
		}
		query += ` AND folder_id=?`
		args = append(args, folderID)
	}

	if keyword != "" {
		query += ` AND (title LIKE ? OR tags_json LIKE ?)`
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}

	query += ` ORDER BY updated_at DESC`

	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文档失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []Document
	for rows.Next() {
		var doc Document
		var tagsJSON string
		if err := rows.Scan(&doc.UUID, &doc.FolderID, &doc.Title, &tagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描文档数据失败: " + err.Error()})
			return
		}
		doc.Tags = tagsFromJSON(tagsJSON)
		list = append(list, doc)
	}

	if list == nil {
		list = []Document{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// GetDocument document details
// GET /api/local/knowledge/documents/:uuid
func (h *Handler) GetDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid 参数不能为空"})
		return
	}

	store, storeErr := h.getStore(c)
	if storeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "获取知识库存储失败: " + storeErr.Error()})
		return
	}

	var doc Document
	var tagsJSON string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at
		FROM gt_knowledge_documents WHERE uuid=? AND deleted=0`, docUUID).
		Scan(&doc.UUID, &doc.FolderID, &doc.Title, &tagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文档失败: " + err.Error()})
		return
	}

	doc.Tags = tagsFromJSON(tagsJSON)

	//Read content from the file system
	content, err := store.ReadFile(docUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文档内容失败: " + err.Error()})
		return
	}
	doc.Content = content
	diskHash := contentHash(content)
	if diskHash != doc.ContentHash {
		doc.ContentHash = diskHash
		doc.WordCount = wordCount(content)
		doc.UpdatedAt = nowMillis()
		_, _ = h.dbRef.Get().ExecContext(c.Request.Context(),
			`UPDATE gt_knowledge_documents SET content_hash=?, word_count=?, updated_at=? WHERE uuid=? AND deleted=0`,
			doc.ContentHash, doc.WordCount, doc.UpdatedAt, docUUID)
	}

	c.JSON(http.StatusOK, doc)
}

// CreateDocument creates a document
// POST /api/local/knowledge/documents
func (h *Handler) CreateDocument(c *gin.Context) {
	store, storeErr := h.getStore(c)
	if storeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "获取知识库存储失败: " + storeErr.Error()})
		return
	}

	var input struct {
		Title    string   `json:"title"`
		FolderID int64    `json:"folder_id"`
		Content  string   `json:"content"`
		Tags     []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if input.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title 不能为空"})
		return
	}

	docUUID := uuid.New().String()
	hash := contentHash(input.Content)
	wc := wordCount(input.Content)
	tagsJSON := tagsToJSON(input.Tags)
	filePath := docUUID + ".md"
	now := nowMillis()

	// write to file
	if err := store.WriteFile(docUUID, input.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败: " + err.Error()})
		return
	}

	//Insert into database
	_, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_knowledge_documents (uuid, folder_id, title, file_path, tags_json, content_hash, word_count, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		docUUID, input.FolderID, input.Title, filePath, tagsJSON, hash, wc, now, now)
	if err != nil {
		//Rollback file
		_ = store.DeleteFile(docUUID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建文档失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, Document{
		UUID:        docUUID,
		FolderID:    input.FolderID,
		Title:       input.Title,
		Tags:        input.Tags,
		ContentHash: hash,
		WordCount:   wc,
		CreatedAt:   now,
		UpdatedAt:   now,
		Content:     input.Content,
	})
}

// UpdateDocument updates the document (automatically saves historical versions)
// PUT /api/local/knowledge/documents/:uuid
func (h *Handler) UpdateDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid 参数不能为空"})
		return
	}

	store, storeErr := h.getStore(c)
	if storeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "获取知识库存储失败: " + storeErr.Error()})
		return
	}

	var input struct {
		Title    *string  `json:"title"`
		FolderID *int64   `json:"folder_id"`
		Content  *string  `json:"content"`
		Tags     []string `json:"tags"`
		BaseHash string   `json:"base_hash"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	// Query existing documents (file content is located by uuid, no need to read the file_path column)
	var doc Document
	var tagsJSON string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at
		FROM gt_knowledge_documents WHERE uuid=? AND deleted=0`, docUUID).
		Scan(&doc.UUID, &doc.FolderID, &doc.Title, &tagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文档失败: " + err.Error()})
		return
	}
	doc.Tags = tagsFromJSON(tagsJSON)

	// Read old content for historical version
	oldContent, err := store.ReadFile(docUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取旧文档内容失败: " + err.Error()})
		return
	}

	//Apply updates
	newContent := oldContent
	contentChanged := false
	if input.Content != nil {
		newContent = *input.Content
		contentChanged = newContent != oldContent
	}
	if input.Title != nil {
		doc.Title = *input.Title
	}
	if input.FolderID != nil {
		doc.FolderID = *input.FolderID
	}
	if input.Tags != nil {
		doc.Tags = input.Tags
	}

	now := nowMillis()
	newHash := doc.ContentHash
	newWC := doc.WordCount

	// If the content changes, save the historical version and write new content
	if contentChanged {
		diskHash := contentHash(oldContent)
		if (input.BaseHash != "" && input.BaseHash != doc.ContentHash) || diskHash != doc.ContentHash {
			c.JSON(http.StatusConflict, gin.H{
				"error":         "文档已被其他程序或页面修改，请重新加载或另存为新文档",
				"code":          "KNOWLEDGE_CONFLICT",
				"base_hash":     input.BaseHash,
				"current_hash":  doc.ContentHash,
				"disk_hash":     diskHash,
				"disk_content":  oldContent,
				"local_content": newContent,
			})
			return
		}

		//Save historical version to database
		histRes, err := h.dbRef.Get().ExecContext(c.Request.Context(),
			`INSERT INTO gt_knowledge_history (doc_uuid, content_hash, word_count, created_at)
			VALUES (?, ?, ?, ?)`,
			docUUID, doc.ContentHash, doc.WordCount, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存历史版本失败: " + err.Error()})
			return
		}
		historyID, err := histRes.LastInsertId()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取历史版本 ID 失败: " + err.Error()})
			return
		}

		//Save old content to history file
		if err := store.WriteHistoryFile(docUUID, historyID, oldContent); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "写入历史文件失败: " + err.Error()})
			return
		}

		//Write new content
		if err := store.WriteFile(docUUID, newContent); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败: " + err.Error()})
			return
		}

		newHash = contentHash(newContent)
		newWC = wordCount(newContent)
	}

	newTagsJSON := tagsToJSON(doc.Tags)

	//Update database
	_, err = h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET folder_id=?, title=?, tags_json=?, content_hash=?, word_count=?, updated_at=?
		WHERE uuid=?`,
		doc.FolderID, doc.Title, newTagsJSON, newHash, newWC, now, docUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新文档失败: " + err.Error()})
		return
	}

	doc.ContentHash = newHash
	doc.WordCount = newWC
	doc.UpdatedAt = now
	doc.Content = newContent

	c.JSON(http.StatusOK, doc)
}

// DeleteDocument deletes the document (moves it to the recycle bin)
// DELETE /api/local/knowledge/documents/:uuid
func (h *Handler) DeleteDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid 参数不能为空"})
		return
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET deleted=1, updated_at=? WHERE uuid=? AND deleted=0`,
		now, docUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文档失败: " + err.Error()})
		return
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取删除影响行数失败: " + err.Error()})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "deleted": true})
}

// ListTrash queries the recycle bin document.
// GET /api/local/knowledge/trash
func (h *Handler) ListTrash(c *gin.Context) {
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at
		FROM gt_knowledge_documents WHERE deleted=1 ORDER BY updated_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询回收站失败: " + err.Error()})
		return
	}
	defer rows.Close()

	list := make([]Document, 0)
	for rows.Next() {
		var doc Document
		var tagsJSON string
		if err := rows.Scan(&doc.UUID, &doc.FolderID, &doc.Title, &tagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描回收站数据失败: " + err.Error()})
			return
		}
		doc.Tags = tagsFromJSON(tagsJSON)
		list = append(list, doc)
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// RestoreDocument restores the document from the recycle bin.
// POST /api/local/knowledge/documents/:uuid/restore
func (h *Handler) RestoreDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET deleted=0, updated_at=? WHERE uuid=? AND deleted=1`,
		now, docUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "恢复文档失败: " + err.Error()})
		return
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取恢复影响行数失败: " + err.Error()})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "回收站中不存在该文档"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "restored": true, "updated_at": now})
}

// HardDeleteDocument completely deletes the document, historical metadata, and local Markdown files.
// DELETE /api/local/knowledge/documents/:uuid/permanent
func (h *Handler) HardDeleteDocument(c *gin.Context) {
	docUUID := c.Param("uuid")

	store, storeErr := h.getStore(c)
	if storeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "获取知识库存储失败: " + storeErr.Error()})
		return
	}

	var exists int
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT 1 FROM gt_knowledge_documents WHERE uuid=? AND deleted=1`, docUUID).Scan(&exists)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "回收站中不存在该文档"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询回收站文档失败: " + err.Error()})
		return
	}

	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开启事务失败: " + err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_knowledge_history WHERE doc_uuid=?`, docUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除历史记录失败: " + err.Error()})
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_knowledge_documents WHERE uuid=? AND deleted=1`, docUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文档记录失败: " + err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交删除事务失败: " + err.Error()})
		return
	}

	if err := store.DeleteFile(docUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := store.DeleteHistoryFiles(docUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "deleted": true, "permanent": true})
}

// ==================== Document History ====================

// ListHistory document historical version list
// GET /api/local/knowledge/documents/:uuid/history
func (h *Handler) ListHistory(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid 参数不能为空"})
		return
	}

	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, doc_uuid, content_hash, word_count, created_at
		FROM gt_knowledge_history WHERE doc_uuid=? ORDER BY created_at DESC`, docUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询历史版本失败: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []History
	for rows.Next() {
		var hist History
		if err := rows.Scan(&hist.ID, &hist.DocUUID, &hist.ContentHash, &hist.WordCount, &hist.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描历史数据失败: " + err.Error()})
			return
		}
		list = append(list, hist)
	}

	if list == nil {
		list = []History{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// GetHistory historical version content
// GET /api/local/knowledge/documents/:uuid/history/:historyId
func (h *Handler) GetHistory(c *gin.Context) {
	docUUID := c.Param("uuid")

	store, storeErr := h.getStore(c)
	if storeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "获取知识库存储失败: " + storeErr.Error()})
		return
	}

	historyIDStr := c.Param("historyId")
	if docUUID == "" || historyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不能为空"})
		return
	}

	historyID, err := strconv.ParseInt(historyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "historyId 参数无效"})
		return
	}

	// Query history records
	var hist History
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, doc_uuid, content_hash, word_count, created_at
		FROM gt_knowledge_history WHERE id=? AND doc_uuid=?`, historyID, docUUID).
		Scan(&hist.ID, &hist.DocUUID, &hist.ContentHash, &hist.WordCount, &hist.CreatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "历史版本不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询历史版本失败: " + err.Error()})
		return
	}

	//Read the contents of the history file
	content, err := store.ReadHistoryFile(docUUID, historyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取历史内容失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           hist.ID,
		"doc_uuid":     hist.DocUUID,
		"content_hash": hist.ContentHash,
		"word_count":   hist.WordCount,
		"created_at":   hist.CreatedAt,
		"content":      content,
	})
}

// ==================== Search ====================

// Search full text search
// GET /api/local/knowledge/search?q=keywords
func (h *Handler) Search(c *gin.Context) {
	store, storeErr := h.getStore(c)
	if storeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "获取知识库存储失败: " + storeErr.Error()})
		return
	}

	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"items": []SearchResult{}, "total": 0})
		return
	}

	// Query all undeleted documents
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, file_path
		FROM gt_knowledge_documents WHERE deleted=0 ORDER BY updated_at DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文档失败: " + err.Error()})
		return
	}
	defer rows.Close()

	type docInfo struct {
		uuid     string
		folderID int64
		title    string
		tagsJSON string
		filePath string
	}
	var docs []docInfo
	for rows.Next() {
		var d docInfo
		if err := rows.Scan(&d.uuid, &d.folderID, &d.title, &d.tagsJSON, &d.filePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描文档数据失败: " + err.Error()})
			return
		}
		docs = append(docs, d)
	}

	lowerQ := strings.ToLower(q)
	var items []SearchResult

	for _, d := range docs {
		// Check if title and tag match
		titleMatched := strings.Contains(strings.ToLower(d.title), lowerQ)
		tagsMatched := strings.Contains(strings.ToLower(d.tagsJSON), lowerQ)

		// Read the file content for full text search
		content, err := store.ReadFile(d.uuid)
		if err != nil {
			continue
		}

		contentMatched := strings.Contains(strings.ToLower(content), lowerQ)

		if !titleMatched && !tagsMatched && !contentMatched {
			continue
		}

		snippet := generateSnippet(content, q, 50)
		if snippet == "" && titleMatched {
			snippet = d.title
		}

		items = append(items, SearchResult{
			UUID:     d.uuid,
			Title:    d.title,
			Snippet:  snippet,
			FolderID: d.folderID,
		})
	}

	if items == nil {
		items = []SearchResult{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// ==================== Auxiliary functions ====================

// buildFolderTree recursively builds the folder tree
func buildFolderTree(folders []FolderNode, parentID int64) []FolderNode {
	var children []FolderNode
	for i := range folders {
		if folders[i].ParentID == parentID {
			folders[i].Children = buildFolderTree(folders, folders[i].ID)
			children = append(children, folders[i])
		}
	}
	return children
}

// contentHash calculates the first 16 bits of SHA-256 of the content
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

// wordCount counts the number of words
func wordCount(content string) int {
	count := 0
	inWord := false
	for _, r := range content {
		switch {
		case unicode.Is(unicode.Han, r):
			count++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				count++
			}
			inWord = true
		case (r == '\'' || r == '’') && inWord:
			// English abbreviations and possessives are still considered the same word.
		default:
			inWord = false
		}
	}
	return count
}

// tagsToJSON serializes the tag array into a JSON string
func tagsToJSON(tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// tagsFromJSON parses the JSON string into an array of tags
func tagsFromJSON(s string) []string {
	var tags []string
	if s == "" {
		return []string{}
	}
	_ = json.Unmarshal([]byte(s), &tags)
	if tags == nil {
		tags = []string{}
	}
	return tags
}

// generateSnippet generates matching snippets, with radius characters before and after
func generateSnippet(content, keyword string, radius int) string {
	contentRunes := []rune(content)
	lowerContent := []rune(strings.ToLower(content))
	lowerKeyword := []rune(strings.ToLower(keyword))
	idx := runeSliceIndex(lowerContent, lowerKeyword)
	if idx == -1 {
		if len(contentRunes) > 100 {
			return string(contentRunes[:100]) + "..."
		}
		return content
	}

	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + len(lowerKeyword) + radius
	if end > len(contentRunes) {
		end = len(contentRunes)
	}

	snippet := string(contentRunes[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(contentRunes) {
		snippet = snippet + "..."
	}
	return snippet
}

func runeSliceIndex(content, keyword []rune) int {
	if len(keyword) == 0 {
		return 0
	}
	if len(keyword) > len(content) {
		return -1
	}
	for i := 0; i <= len(content)-len(keyword); i++ {
		matched := true
		for j := range keyword {
			if content[i+j] != keyword[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}
