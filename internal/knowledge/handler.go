package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"goteams-client/internal/applog"
	"goteams-client/internal/i18n"
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
	FilePath    string   `json:"file_path"`
	Ext         string   `json:"ext"`
	Tags        []string `json:"tags"`
	Content     string   `json:"content,omitempty"`
	TagsJSON    string   `json:"-"`
	ContentHash string   `json:"content_hash,omitempty"`
	WordCount   int      `json:"word_count,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
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
	defaultRootDir     func() string
	mu                 sync.Mutex
	stores             map[string]*Store
	scanMu             sync.Mutex // 串行化 Scan，保证并发幂等（BE-04）
}

// NewHandler creates a knowledge base handler
// contentDirProvider resolves the KR (knowledge root directory) for the current Profile.
// defaultRootDir returns the default KR (used by GetRoot for the breadcrumb default).
func NewHandler(dbRef *storage.DBRef, contentDirProvider func() (string, error), defaultRootDir func() string) *Handler {
	return &Handler{
		dbRef:              dbRef,
		contentDirProvider: contentDirProvider,
		defaultRootDir:     defaultRootDir,
		stores:             make(map[string]*Store),
	}
}

// getStore parses file storage according to the current KR and caches it per KR directory.
// On first creation for a KR, triggers a one-time legacy content migration (DA-03).
func (h *Handler) getStore(c *gin.Context) (*Store, error) {
	if h.contentDirProvider == nil {
		return nil, fmt.Errorf("未配置知识库存储")
	}
	krDir, err := h.contentDirProvider()
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	s, ok := h.stores[krDir]
	h.mu.Unlock()
	if !ok {
		s, err = NewStore(krDir)
		if err != nil {
			return nil, err
		}
		// S-DA-03: 首次为该 KR 创建存储时触发旧数据一次性迁移（失败不阻断）
		if merr := MigrateLegacyContent(krDir, filepath.Dir(filepath.Dir(krDir)), h.dbRef.Get()); merr != nil {
			applog.Warn("知识库旧数据迁移失败（不影响启动）", "krDir", krDir, "error", merr)
		}
		h.mu.Lock()
		h.stores[krDir] = s
		h.mu.Unlock()
	}
	return s, nil
}

// nowMillis returns the current Unix millisecond timestamp
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// ==================== KR config (BE-01 / DA-01) ====================

// GetRoot returns the current KR and the default KR.
// GET /api/local/knowledge/root
func (h *Handler) GetRoot(c *gin.Context) {
	root, err := h.contentDirProvider()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	def := ""
	if h.defaultRootDir != nil {
		def = h.defaultRootDir()
	}
	// S-DA-01: 返回当前 KR 与默认 KR（验收②③）
	c.JSON(http.StatusOK, gin.H{"root": root, "default": def})
}

// PutRoot migrates the knowledge base content to a new KR (S-BE-07 ~ S-BE-14 / S-IN-06).
// PUT /api/local/knowledge/root  body {dir, mode}
//
// S-IN-06: mode 为必填字段，当前仅支持 "migrate"；缺省或非法一律 400，
// 不静默沿用旧的「仅切换配置指向」语义。
func (h *Handler) PutRoot(c *gin.Context) {
	var input struct {
		Dir  string `json:"dir"`
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Dir) == "" {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	dir := strings.TrimSpace(input.Dir)
	// S-BE-13: 模式非法返回可区分的错误码，禁止回落到模糊的「参数无效」
	if strings.TrimSpace(input.Mode) != migrateMode {
		i18n.Error(c, http.StatusBadRequest, "knowledge_root_mode_invalid", "knowledge_root_mode_invalid")
		return
	}

	// S-BE-14：迁移（含旧 KR 解析、目标校验与配置写入）必须整体处于同一串行域内。
	// oldRoot 的解析绝不能前置到加锁之前：并发两次迁移若都基于同一个旧 KR 规划，
	// 先成功者的迁移结果会被后到者的「空计划 + 重写配置」覆盖，导致
	// knowledge_root_dir 指向空目录而真实内容留在第一个目标（违反 S-DA-08）。
	h.scanMu.Lock()
	defer h.scanMu.Unlock()

	oldRoot, err := h.contentDirProvider()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	// S-BE-10: 目标为当前 KR 自身或其子目录 -> 拒绝，且不做任何文件操作
	// （必须早于 checkDirWritable，后者会创建目标目录）
	if isSameOrSubPath(oldRoot, dir) {
		i18n.Error(c, http.StatusBadRequest, "knowledge_root_target_invalid", "knowledge_root_target_invalid")
		return
	}
	// S-DA-01: 校验目标可写，不可写则原配置不变（验收① / BE-01）
	if err := checkDirWritable(dir); err != nil {
		i18n.Error(c, http.StatusBadRequest, "knowledge_root_not_writable", "knowledge_root_not_writable")
		return
	}

	// S-BE-08 / S-IN-07: 顶层同名冲突预检；冲突即中止迁移，且配置保持原值
	plan, conflicts, perr := buildMigratePlan(oldRoot, dir)
	if perr != nil {
		applog.Warn("知识库目录迁移预检失败", "old_root", oldRoot, "new_root", dir, "error", perr)
		writeMigrateFailed(c, migrateRollbackResult{Completed: true, Failed: []string{}})
		return
	}
	if len(conflicts) > 0 {
		// S-IN-07: 冲突清单落在错误响应顶层，供前端直接渲染（不覆盖、不自动改名）
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{
			"error":     i18n.T(c, "knowledge_root_migrate_conflict"),
			"code":      "knowledge_root_migrate_conflict",
			"conflicts": conflicts,
		})
		return
	}

	// S-BE-07 / S-BE-11 / S-DA-07: 执行 move（含 .trash / .history），旧目录不再保留内容
	journal, merr := executeMigrate(plan)
	if merr != nil {
		// S-BE-09: 逆序回滚已移动项；配置尚未写入，原值天然保持
		rollback := rollbackMigrate(journal)
		applog.Warn("知识库目录迁移失败", "old_root", oldRoot, "new_root", dir, "error", merr)
		writeMigrateFailed(c, rollback)
		return
	}

	// S-BE-09 / S-DA-08: 配置写入严格后置于迁移全部成功之后，避免「配置指向半成品目录」
	if err := SetRootDir(h.dbRef.Get(), dir); err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_root_set_failed", "knowledge_root_set_failed")
		return
	}
	// 重置旧 KR 与新 KR 的 store 缓存，确保后续请求使用新目录
	h.mu.Lock()
	delete(h.stores, oldRoot)
	delete(h.stores, dir)
	h.mu.Unlock()
	// E18 / AC-5: 旧 KR 已迁空则移除该空目录（仅在配置写入成功后清理；
	// 上面 writeMigrateFailed 的失败回滚路径绝不清理）。清理失败不影响本次成功判定。
	pruneEmptyDir(oldRoot)
	// S-BE-12: 迁移成功后重建新目录索引（调用无锁版本，避免与 scanMu 自死锁，CF-2）
	var scanWarning string
	if _, _, serr := h.runScanLocked(c.Request.Context(), dir); serr != nil {
		applog.Warn("迁移知识库目录后扫描失败", "root", dir, "error", serr)
		// E18 取代 S-IN-06 / AC-7：迁移与配置写入均已成功，HTTP 语义保持 200 不变，
		// 但重扫失败必须对用户可见（原先仅 Warn 后 200，用户完全不可感知），
		// 故追加可选 warning 字段供前端非阻断提示。文案沿用仓库既有 gin.H["warning"] 约定。
		scanWarning = i18n.T(c, "knowledge_root_scan_warning")
	}

	// S-IN-06: 成功响应返回新 root 与迁移统计（重扫正常时不携带 warning，行为与现状逐字一致）
	resp := gin.H{"root": dir, "migrated": plan.stats}
	if scanWarning != "" {
		resp["warning"] = scanWarning
	}
	c.JSON(http.StatusOK, resp)
}

// writeMigrateFailed 输出迁移失败响应：500 + 可区分错误码 + rollback 结构（S-BE-09 / S-BE-13）。
func writeMigrateFailed(c *gin.Context, rollback migrateRollbackResult) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"error":    i18n.T(c, "knowledge_root_migrate_failed"),
		"code":     "knowledge_root_migrate_failed",
		"rollback": rollback,
	})
}

// ==================== Folder management ====================

// ListFolders lists all folders (tree shape)
// GET /api/local/knowledge/folders
func (h *Handler) ListFolders(c *gin.Context) {
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, name, parent_id, created_at, updated_at FROM gt_knowledge_folders ORDER BY id ASC`)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer rows.Close()

	var folders []FolderNode
	for rows.Next() {
		var f FolderNode
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
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
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	if input.Name == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_name_required", "")
		return
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_knowledge_folders (name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		input.Name, input.ParentID, now, now)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
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
		i18n.Error(c, http.StatusBadRequest, "common_invalid_id", "")
		return
	}

	var input struct {
		Name     *string `json:"name"`
		ParentID *int64  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}

	var name string
	var parentID int64
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT name, parent_id FROM gt_knowledge_folders WHERE id=?`, id).Scan(&name, &parentID)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_folder_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	// Prevent folders from being moved under itself or its descendants.
	if input.ParentID != nil && *input.ParentID != parentID {
		if *input.ParentID == id {
			i18n.Error(c, http.StatusBadRequest, "knowledge_folder_self_parent", "")
			return
		}
		if *input.ParentID != 0 {
			descendant, err := h.isDescendantFolder(c.Request.Context(), *input.ParentID, id)
			if err != nil {
				i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
				return
			}
			if descendant {
				i18n.Error(c, http.StatusBadRequest, "knowledge_folder_descendant", "")
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
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "updated_at": now})
}

// isDescendantFolder determines whether candidateID is a descendant of ancestorID.
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

// DeleteFolder deletes a folder with keep/purge semantics (BE-03 / US3).
// DELETE /api/local/knowledge/folders/:id  body {mode:"keep"|"purge"}
func (h *Handler) DeleteFolder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		i18n.Error(c, http.StatusBadRequest, "common_invalid_id", "")
		return
	}

	// S-IN-02: mode 缺省或非法必须显式拒绝（旧调用方须升级，不静默沿用）
	var input struct {
		Mode string `json:"mode"`
	}
	if b, berr := c.GetRawData(); berr == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &input)
	}
	if input.Mode != "keep" && input.Mode != "purge" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_folder_mode_invalid", "")
		return
	}

	var parentID int64
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT parent_id FROM gt_knowledge_folders WHERE id=?`, id).Scan(&parentID)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_folder_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	descendants, derr := h.collectDescendantFolders(c.Request.Context(), id)
	if derr != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	inIDs, inArgs := inClause(descendants)

	if input.Mode == "keep" {
		// S-BE-03: keep = 子树内所有文档上提到父级（无父则根），子文件夹随之上提
		tx, terr := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
		if terr != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		defer tx.Rollback()
		docArgs := append([]interface{}{parentID}, inArgs...)
		if _, e := tx.ExecContext(c.Request.Context(),
			fmt.Sprintf(`UPDATE gt_knowledge_documents SET folder_id=? WHERE folder_id IN (%s)`, inIDs), docArgs...); e != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_folder_move_docs_failed", "")
			return
		}
		if _, e := tx.ExecContext(c.Request.Context(),
			`UPDATE gt_knowledge_folders SET parent_id=? WHERE parent_id=?`, parentID, id); e != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_folder_move_children_failed", "")
			return
		}
		if _, e := tx.ExecContext(c.Request.Context(),
			`DELETE FROM gt_knowledge_folders WHERE id=?`, id); e != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_folder_delete_failed", "")
			return
		}
		if e := tx.Commit(); e != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_delete_transaction_failed", "")
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": id, "mode": "keep", "deleted": true})
		return
	}

	// S-BE-03: purge = 文档软删并移入回收站；文件夹及子文件夹从 DB 与 KR 删除
	krDir, kerr := h.contentDirProvider()
	if kerr != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	store, serr := NewStore(krDir)
	if serr != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	// 1) 先把子树内真实文档移入回收站（事务外，先成功再做 DB 变更）
	docRows, rerr := h.dbRef.Get().QueryContext(c.Request.Context(),
		fmt.Sprintf(`SELECT uuid, file_path FROM gt_knowledge_documents WHERE folder_id IN (%s) AND deleted=0`, inIDs), inArgs...)
	if rerr == nil {
		for docRows.Next() {
			var docUUID, filePath string
			if sErr := docRows.Scan(&docUUID, &filePath); sErr != nil {
				continue
			}
			if _, mErr := store.MoveToTrash(filePath); mErr != nil {
				applog.Warn("删除文件夹时移入回收站失败，跳过", "uuid", docUUID, "error", mErr)
			}
		}
		docRows.Close()
	}
	// 2) 删除 KR 内真实文件夹目录（文档已移入回收站）
	for _, fid := range descendants {
		if rel, rerr := h.folderRelPath(c.Request.Context(), fid); rerr == nil && rel != "" {
			if full, perr := store.DocPath(rel); perr == nil {
				if rmErr := os.RemoveAll(full); rmErr != nil {
					applog.Warn("删除真实文件夹目录失败，跳过", "rel", rel, "error", rmErr)
				}
			}
		}
	}
	// 3) 事务内软删文档 + 删除文件夹节点（要么全成功要么回滚）
	tx, terr := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if terr != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer tx.Rollback()
	delArgs := append([]interface{}{nowMillis()}, inArgs...)
	if _, e := tx.ExecContext(c.Request.Context(),
		fmt.Sprintf(`UPDATE gt_knowledge_documents SET deleted=1, updated_at=? WHERE folder_id IN (%s)`, inIDs), delArgs...); e != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_record_delete_failed", "")
		return
	}
	if _, e := tx.ExecContext(c.Request.Context(),
		fmt.Sprintf(`DELETE FROM gt_knowledge_folders WHERE id IN (%s)`, inIDs), inArgs...); e != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_folder_delete_failed", "")
		return
	}
	if e := tx.Commit(); e != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_delete_transaction_failed", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "mode": "purge", "deleted": true})
}

// ==================== Document Management ====================

// ListDocuments document list
// GET /api/local/knowledge/documents?folder_id=X&keyword=Y
func (h *Handler) ListDocuments(c *gin.Context) {
	folderIDStr := c.Query("folder_id")
	keyword := c.Query("keyword")

	query := `SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at, file_path, ext
		FROM gt_knowledge_documents WHERE deleted=0`
	var args []interface{}

	if folderIDStr != "" {
		folderID, err := strconv.ParseInt(folderIDStr, 10, 64)
		if err != nil {
			i18n.Error(c, http.StatusBadRequest, "knowledge_folder_id_invalid", "")
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
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer rows.Close()

	var list []Document
	for rows.Next() {
		var doc Document
		var tagsJSON string
		if err := rows.Scan(&doc.UUID, &doc.FolderID, &doc.Title, &doc.TagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt, &doc.FilePath, &doc.Ext); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
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
		i18n.Error(c, http.StatusBadRequest, "knowledge_uuid_required", "")
		return
	}

	store, storeErr := h.getStore(c)
	if storeErr != nil {
		i18n.Error(c, http.StatusUnauthorized, "common_server_error", "")
		return
	}

	var doc Document
	var tagsJSON, filePath, ext string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at, file_path, ext
		FROM gt_knowledge_documents WHERE uuid=? AND deleted=0`, docUUID).
		Scan(&doc.UUID, &doc.FolderID, &doc.Title, &doc.TagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt, &filePath, &ext)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_document_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	doc.Tags = tagsFromJSON(tagsJSON)
	doc.FilePath = filePath
	doc.Ext = ext

	// S-DA-02: 正文按 file_path 从 KR 真实文件读取
	content, err := store.ReadFile(filePath)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_content_read_failed", err.Error())
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

// CreateDocument creates a document, optionally as md/txt (BE-02 / IN-02).
// POST /api/local/knowledge/documents  body {title, folder_id, content, tags, ext?}
func (h *Handler) CreateDocument(c *gin.Context) {
	store, storeErr := h.getStore(c)
	if storeErr != nil {
		i18n.Error(c, http.StatusUnauthorized, "common_server_error", "")
		return
	}

	var input struct {
		Title    string   `json:"title"`
		FolderID int64    `json:"folder_id"`
		Content  string   `json:"content"`
		Tags     []string `json:"tags"`
		Ext      string   `json:"ext"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	if input.Title == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_title_required", "")
		return
	}

	// S-BE-02: ext 在 md/txt 间选择，默认 md（验收①）
	ext := strings.ToLower(strings.TrimSpace(input.Ext))
	if ext == "" {
		ext = "md"
	}
	if ext != "md" && ext != "txt" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_document_ext_invalid", "")
		return
	}

	// S-DA-02: 按 folder_id 解析真实相对目录
	folderRel, ferr := h.folderRelPath(c.Request.Context(), input.FolderID)
	if ferr != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	// S-BE-02: 标题非法字符安全转义为文件名（验收②）
	fileName := SanitizeFileName(input.Title) + "." + ext
	relPath := fileName
	if folderRel != "" {
		relPath = folderRel + "/" + fileName
	}
	// S-DA-02: 同名冲突自动加 (n) 后缀，避免覆盖（验收③）
	relPath = h.uniqueNewRelPath(c.Request.Context(), store, relPath)

	docUUID := uuid.New().String()
	hash := contentHash(input.Content)
	wc := wordCount(input.Content)
	tagsJSON := tagsToJSON(input.Tags)
	now := nowMillis()

	// S-DA-02: 写 KR 内真实文件（标题命名）
	if err := store.WriteFile(relPath, input.Content); err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_file_write_failed", "")
		return
	}

	_, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`INSERT INTO gt_knowledge_documents (uuid, folder_id, title, file_path, ext, tags_json, content_hash, word_count, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		docUUID, input.FolderID, input.Title, relPath, ext, tagsJSON, hash, wc, now, now)
	if err != nil {
		_ = store.DeleteFile(relPath)
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_create_failed", "")
		return
	}

	doc := Document{
		UUID:        docUUID,
		FolderID:    input.FolderID,
		Title:       input.Title,
		FilePath:    relPath,
		Ext:         ext,
		Tags:        input.Tags,
		ContentHash: hash,
		WordCount:   wc,
		CreatedAt:   now,
		UpdatedAt:   now,
		Content:     input.Content,
	}
	c.JSON(http.StatusCreated, doc)
}

// UpdateDocument updates the document, moving the real file when title/folder changes (UI-04).
// PUT /api/local/knowledge/documents/:uuid
func (h *Handler) UpdateDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_uuid_required", "")
		return
	}

	store, storeErr := h.getStore(c)
	if storeErr != nil {
		i18n.ErrorDev(c, http.StatusUnauthorized, "common_server_error", "", storeErr)
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
		i18n.ErrorDev(c, http.StatusBadRequest, "common_request_invalid", "", err)
		return
	}

	var doc Document
	var tagsJSON, filePath, ext string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at, file_path, ext
		FROM gt_knowledge_documents WHERE uuid=? AND deleted=0`, docUUID).
		Scan(&doc.UUID, &doc.FolderID, &doc.Title, &doc.TagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt, &filePath, &ext)
	if err == sql.ErrNoRows {
		i18n.ErrorDev(c, http.StatusNotFound, "knowledge_document_not_found", "", err)
		return
	}
	if err != nil {
		i18n.ErrorDev(c, http.StatusInternalServerError, "common_server_error", "", err)
		return
	}
	doc.Tags = tagsFromJSON(tagsJSON)

	// S-DA-02: 正文按 file_path 从 KR 真实文件读取
	oldContent, err := store.ReadFile(filePath)
	if err != nil {
		i18n.ErrorDev(c, http.StatusInternalServerError, "knowledge_old_content_read_failed", "", err)
		return
	}

	newContent := oldContent
	contentChanged := false
	if input.Content != nil {
		newContent = *input.Content
		contentChanged = newContent != oldContent
	}
	newTitle := doc.Title
	newFolderID := doc.FolderID
	if input.Title != nil {
		newTitle = *input.Title
	}
	if input.FolderID != nil {
		newFolderID = *input.FolderID
	}

	// S-DA-02: 标题/文件夹变更导致 file_path 变化时，重命名真实文件
	extEffective := ext
	if extEffective == "" {
		extEffective = "md"
	}
	// S-DA-02: file_path 统一使用正斜杠存储，避免 Windows/Unix 路径符不一致导致误判路径变化
	filePathNorm := normalizePath(filePath)
	newFilePath := filePathNorm
	if input.Title != nil || input.FolderID != nil {
		folderRel, ferr := h.folderRelPath(c.Request.Context(), newFolderID)
		if ferr != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		newFileName := SanitizeFileName(newTitle) + "." + extEffective
		cand := newFileName
		if folderRel != "" {
			cand = folderRel + "/" + newFileName
		}
		if !pathsEqual(cand, filePathNorm) {
			// S-DA-02: 避免覆盖同名文件
			newFilePath = h.moveRelPathUnique(c.Request.Context(), store, cand, docUUID)
		}
	}
	if input.Tags != nil {
		doc.Tags = input.Tags
	}

	now := nowMillis()
	newHash := doc.ContentHash
	newWC := doc.WordCount

	// UI-04 验收④：失败时按反序回滚已成功的副作用，避免 DB 与磁盘分裂。
	// 二次回滚失败必须 applog.Error，不能让主流程的错误被吞。
	committed := false
	rollbackCtx := c.Request.Context()
	var rollbacks []func()
	defer func() {
		if committed {
			return
		}
		for i := len(rollbacks) - 1; i >= 0; i-- {
			rollbacks[i]()
		}
	}()

	if contentChanged {
		diskHash := contentHash(oldContent)
		if (input.BaseHash != "" && input.BaseHash != doc.ContentHash) || diskHash != doc.ContentHash {
			c.JSON(http.StatusConflict, gin.H{
				"error":         i18n.T(c, "knowledge_document_conflict"),
				"code":          "KNOWLEDGE_CONFLICT",
				"base_hash":     input.BaseHash,
				"current_hash":  doc.ContentHash,
				"disk_hash":     diskHash,
				"disk_content":  oldContent,
				"local_content": newContent,
			})
			return
		}

		// 步骤1：保存历史版本行。LastInsertId 失败时按三元组兜底删除刚插入的行。
		histRes, err := h.dbRef.Get().ExecContext(c.Request.Context(),
			`INSERT INTO gt_knowledge_history (doc_uuid, content_hash, word_count, created_at)
			VALUES (?, ?, ?, ?)`,
			docUUID, doc.ContentHash, doc.WordCount, now)
		if err != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_history_save_failed", "")
			return
		}
		historyID, err := histRes.LastInsertId()
		if err != nil {
			_, _ = h.dbRef.Get().ExecContext(c.Request.Context(),
				`DELETE FROM gt_knowledge_history
				WHERE doc_uuid=? AND created_at=? AND content_hash=? AND word_count=?`,
				docUUID, now, doc.ContentHash, doc.WordCount)
			i18n.Error(c, http.StatusInternalServerError, "knowledge_history_id_failed", "")
			return
		}
		rollbacks = append(rollbacks, func() {
			if _, derr := h.dbRef.Get().ExecContext(rollbackCtx,
				`DELETE FROM gt_knowledge_history WHERE id=?`, historyID); derr != nil {
				applog.Error("知识库更新回滚失败：删除历史行", "uuid", docUUID, "history_id", historyID, "error", derr)
			}
		})

		// 步骤2：写历史版本文件。
		if err := store.WriteHistoryFile(docUUID, historyID, oldContent); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_history_file_write_failed", "")
			return
		}
		rollbacks = append(rollbacks, func() {
			if ferr := store.DeleteFile(store.HistoryPath(docUUID, historyID)); ferr != nil {
				applog.Error("知识库更新回滚失败：删除历史文件", "uuid", docUUID, "history_id", historyID, "error", ferr)
			}
		})

		// 步骤3：移动真实文件（标题/文件夹变化时）。
		if !pathsEqual(newFilePath, filePath) {
			if err := store.MoveFile(filePath, newFilePath); err != nil {
				i18n.Error(c, http.StatusInternalServerError, "knowledge_file_write_failed", "")
				return
			}
			oldPathForRollback := filePath
			rollbacks = append(rollbacks, func() {
				if mferr := store.MoveFile(newFilePath, oldPathForRollback); mferr != nil {
					applog.Error("知识库更新回滚失败：反向移动文件", "uuid", docUUID, "old", oldPathForRollback, "new", newFilePath, "error", mferr)
				}
			})
		}

		// 步骤4：写入新内容。
		if err := store.WriteFile(newFilePath, newContent); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_file_write_failed", "")
			return
		}
		rollbacks = append(rollbacks, func() {
			if wferr := store.WriteFile(newFilePath, oldContent); wferr != nil {
				applog.Error("知识库更新回滚失败：写回旧内容", "uuid", docUUID, "path", newFilePath, "error", wferr)
			}
		})

		newHash = contentHash(newContent)
		newWC = wordCount(newContent)
	} else if !pathsEqual(newFilePath, filePath) {
		// 仅标题/文件夹变更，内容未变：移动文件
		if err := store.MoveFile(filePath, newFilePath); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "knowledge_file_write_failed", "")
			return
		}
		oldPathForRollback := filePath
		rollbacks = append(rollbacks, func() {
			if mferr := store.MoveFile(newFilePath, oldPathForRollback); mferr != nil {
				applog.Error("知识库更新回滚失败：反向移动文件", "uuid", docUUID, "old", oldPathForRollback, "new", newFilePath, "error", mferr)
			}
		})
	}

	// 步骤5：最后写 DB；失败时 defer 会按反序回滚已成功的副作用。
	newTagsJSON := tagsToJSON(doc.Tags)
	_, err = h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET folder_id=?, title=?, file_path=?, ext=?, tags_json=?, content_hash=?, word_count=?, updated_at=?
		WHERE uuid=?`,
		newFolderID, newTitle, newFilePath, extEffective, newTagsJSON, newHash, newWC, now, docUUID)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_update_failed", err.Error())
		return
	}
	committed = true

	doc.FolderID = newFolderID
	doc.Title = newTitle
	doc.FilePath = newFilePath
	doc.Ext = extEffective
	doc.ContentHash = newHash
	doc.WordCount = newWC
	doc.UpdatedAt = now
	doc.Content = newContent

	c.JSON(http.StatusOK, doc)
}

// DeleteDocument deletes the document (soft delete + move real file to trash, DA-04).
// DELETE /api/local/knowledge/documents/:uuid
func (h *Handler) DeleteDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_uuid_required", "")
		return
	}

	var filePath string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT file_path FROM gt_knowledge_documents WHERE uuid=? AND deleted=0`, docUUID).Scan(&filePath)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_document_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET deleted=1, updated_at=? WHERE uuid=? AND deleted=0`,
		now, docUUID)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_delete_failed", "")
		return
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	if rowsAffected == 0 {
		i18n.Error(c, http.StatusNotFound, "knowledge_document_not_found", "")
		return
	}

	// S-DA-04: 真实文件移入回收站（best-effort，文件夹缺失不影响软删结果）
	if krDir, kerr := h.contentDirProvider(); kerr == nil {
		if st, serr := NewStore(krDir); serr == nil {
			if _, merr := st.MoveToTrash(filePath); merr != nil {
				applog.Warn("删除文档时移入回收站失败", "uuid", docUUID, "error", merr)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "deleted": true})
}

// ListTrash queries the recycle bin document.
// GET /api/local/knowledge/trash
func (h *Handler) ListTrash(c *gin.Context) {
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, content_hash, word_count, created_at, updated_at, file_path, ext
		FROM gt_knowledge_documents WHERE deleted=1 ORDER BY updated_at DESC`)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer rows.Close()

	list := make([]Document, 0)
	for rows.Next() {
		var doc Document
		var tagsJSON, filePath, ext string
		if err := rows.Scan(&doc.UUID, &doc.FolderID, &doc.Title, &doc.TagsJSON,
			&doc.ContentHash, &doc.WordCount, &doc.CreatedAt, &doc.UpdatedAt, &filePath, &ext); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		doc.Tags = tagsFromJSON(tagsJSON)
		doc.FilePath = filePath
		doc.Ext = ext
		list = append(list, doc)
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// RestoreDocument restores the document from the recycle bin (DA-04).
// POST /api/local/knowledge/documents/:uuid/restore
func (h *Handler) RestoreDocument(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_uuid_required", "")
		return
	}

	var filePath string
	var folderID int64
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT file_path, folder_id FROM gt_knowledge_documents WHERE uuid=? AND deleted=1`, docUUID).
		Scan(&filePath, &folderID)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_trash_document_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	now := nowMillis()
	res, err := h.dbRef.Get().ExecContext(c.Request.Context(),
		`UPDATE gt_knowledge_documents SET deleted=0, updated_at=? WHERE uuid=? AND deleted=1`,
		now, docUUID)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_restore_failed", "")
		return
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	if rowsAffected == 0 {
		i18n.Error(c, http.StatusNotFound, "knowledge_trash_document_not_found", "")
		return
	}

	// S-DA-04: 真实文件从回收站移回原 file_path（文件夹缺失则重建目录）
	if krDir, kerr := h.contentDirProvider(); kerr == nil {
		if st, serr := NewStore(krDir); serr == nil {
			if full, perr := st.DocPath(filePath); perr == nil {
				_ = os.MkdirAll(filepath.Dir(full), 0755)
			}
			name := filepath.Base(filePath)
			if trashRel, ferr := st.FindInTrash(name); ferr == nil && trashRel != "" {
				if merr := st.RestoreFromTrash(trashRel, filePath); merr != nil {
					applog.Warn("恢复文档时移出回收站失败", "uuid", docUUID, "error", merr)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "restored": true, "updated_at": now})
}

// HardDeleteDocument permanently deletes the document and its real files (DA-04).
// DELETE /api/local/knowledge/documents/:uuid/permanent
func (h *Handler) HardDeleteDocument(c *gin.Context) {
	docUUID := c.Param("uuid")

	var filePath string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT file_path FROM gt_knowledge_documents WHERE uuid=? AND deleted=1`, docUUID).Scan(&filePath)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_trash_document_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	tx, err := h.dbRef.Get().BeginTx(c.Request.Context(), nil)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_knowledge_history WHERE doc_uuid=?`, docUUID); err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_history_delete_failed", "")
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(),
		`DELETE FROM gt_knowledge_documents WHERE uuid=? AND deleted=1`, docUUID); err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_document_record_delete_failed", "")
		return
	}
	if err := tx.Commit(); err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	// S-DA-04: 删除 .trash 内真实文件与历史文件
	if krDir, kerr := h.contentDirProvider(); kerr == nil {
		if st, serr := NewStore(krDir); serr == nil {
			name := filepath.Base(filePath)
			if trashRel, ferr := st.FindInTrash(name); ferr == nil && trashRel != "" {
				_ = st.DeleteFile(trashRel)
			}
			_ = st.DeleteHistoryFiles(docUUID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "deleted": true, "permanent": true})
}

// ==================== Document History ====================

// ListHistory document historical version list
// GET /api/local/knowledge/documents/:uuid/history
func (h *Handler) ListHistory(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_uuid_required", "")
		return
	}

	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, doc_uuid, content_hash, word_count, created_at
		FROM gt_knowledge_history WHERE doc_uuid=? ORDER BY created_at DESC`, docUUID)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer rows.Close()

	var list []History
	for rows.Next() {
		var hist History
		if err := rows.Scan(&hist.ID, &hist.DocUUID, &hist.ContentHash, &hist.WordCount, &hist.CreatedAt); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
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
		i18n.Error(c, http.StatusUnauthorized, "common_server_error", "")
		return
	}

	historyIDStr := c.Param("historyId")
	if docUUID == "" || historyIDStr == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_parameter_required", "")
		return
	}

	historyID, err := strconv.ParseInt(historyIDStr, 10, 64)
	if err != nil {
		i18n.Error(c, http.StatusBadRequest, "knowledge_history_id_invalid", "")
		return
	}

	var hist History
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, doc_uuid, content_hash, word_count, created_at
		FROM gt_knowledge_history WHERE id=? AND doc_uuid=?`, historyID, docUUID).
		Scan(&hist.ID, &hist.DocUUID, &hist.ContentHash, &hist.WordCount, &hist.CreatedAt)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "knowledge_history_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	// S-DA-04: 历史版本按 HistoryPath 读取（uuid 维度，保持不变）
	content, err := store.ReadHistoryFile(docUUID, historyID)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_history_content_read_failed", "")
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

// ==================== Scan (BE-04 / DA-05) ====================

// Scan triggers a KR directory scan and indexes folders / md / txt (BE-04 / DA-05).
// POST /api/local/knowledge/scan
func (h *Handler) Scan(c *gin.Context) {
	krDir, err := h.contentDirProvider()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	addedFolders, addedDocs, serr := h.runScan(c.Request.Context(), krDir)
	if serr != nil {
		i18n.Error(c, http.StatusInternalServerError, "knowledge_scan_failed", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"added_folders": addedFolders, "added_documents": addedDocs})
}

// runScan performs the KR scan under a single-flight lock to guarantee idempotency (BE-04 验收).
func (h *Handler) runScan(ctx context.Context, krDir string) (int, int, error) {
	h.scanMu.Lock()
	defer h.scanMu.Unlock()
	return h.runScanLocked(ctx, krDir)
}

// runScanLocked 执行 KR 扫描，且不自持 scanMu（S-BE-14 / CF-2）。
// 调用方须已持有 scanMu：runScan 负责加锁；目录迁移在全过程持锁期间直接调用本方法，
// 从而与扫描共用同一串行域而不产生自死锁。内部逻辑与既有 runScan 完全一致。
func (h *Handler) runScanLocked(ctx context.Context, krDir string) (int, int, error) {
	// S-DA-03: 扫描前先确保旧数据已迁移（幂等、失败不阻断）
	if merr := MigrateLegacyContent(krDir, filepath.Dir(filepath.Dir(krDir)), h.dbRef.Get()); merr != nil {
		applog.Warn("知识库旧数据迁移失败（不影响扫描）", "krDir", krDir, "error", merr)
	}

	store, err := NewStore(krDir)
	if err != nil {
		return 0, 0, err
	}
	dirToID, addedFolders, err := h.scanFolders(ctx, store)
	if err != nil {
		return addedFolders, 0, err
	}
	addedDocs, err := h.scanDocuments(ctx, store, dirToID)
	if err != nil {
		return addedFolders, addedDocs, err
	}
	return addedFolders, addedDocs, nil
}

// scanFolders walks the KR and ensures gt_knowledge_folders records exist for real directories.
// Returns a map from relative dir path -> folder id for document resolution.
func (h *Handler) scanFolders(ctx context.Context, store *Store) (map[string]int64, int, error) {
	krDir := store.KRDir()
	dirToID := make(map[string]int64)
	count := 0
	err := filepath.WalkDir(krDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(krDir, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		// S-DA-05: 排除 .trash / .history 及隐藏目录
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		parentRel := filepath.Dir(rel)
		parentID := int64(0)
		if parentRel != "." {
			if id, ok := dirToID[parentRel]; ok {
				parentID = id
			}
		}
		id, created, cerr := h.ensureFolder(ctx, d.Name(), parentID)
		if cerr != nil {
			return cerr
		}
		dirToID[rel] = id
		if created {
			count++
		}
		return nil
	})
	return dirToID, count, err
}

// scanDocuments walks the KR and indexes real .md/.txt files not yet indexed (DA-05 验收①③④).
func (h *Handler) scanDocuments(ctx context.Context, store *Store, dirToID map[string]int64) (int, error) {
	krDir := store.KRDir()
	count := 0
	err := filepath.WalkDir(krDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// S-DA-05: 排除 .trash / .history 及隐藏目录
			if d.Name() == ".trash" || d.Name() == ".history" || (path != krDir && strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		rel, rerr := filepath.Rel(krDir, path)
		if rerr != nil {
			return rerr
		}
		// S-DA-02: file_path 统一使用正斜杠存储，避免 Windows/Unix 路径符不一致
		relNorm := normalizePath(rel)
		// S-DA-05: 按规范化后的相对路径去重，已索引则跳过
		var dummy int
		scanErr := h.dbRef.Get().QueryRowContext(ctx,
			`SELECT 1 FROM gt_knowledge_documents WHERE REPLACE(file_path, '\\', '/')=?`, relNorm).Scan(&dummy)
		if scanErr == nil {
			return nil
		}
		parentRel := normalizePath(filepath.Dir(rel))
		folderID := int64(0)
		if parentRel != "." && parentRel != "" {
			if id, ok := dirToID[parentRel]; ok {
				folderID = id
			}
		}
		title := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		now := nowMillis()
		_, ierr := h.dbRef.Get().ExecContext(ctx,
			`INSERT INTO gt_knowledge_documents (uuid, folder_id, title, file_path, ext, tags_json, content_hash, word_count, created_at, updated_at, deleted)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			uuid.New().String(), folderID, title, relNorm, ext[1:], "[]", "", 0, now, now)
		if ierr != nil {
			return ierr
		}
		count++
		return nil
	})
	return count, err
}

// ensureFolder finds or creates a folder record by (name, parent_id).
func (h *Handler) ensureFolder(ctx context.Context, name string, parentID int64) (int64, bool, error) {
	var id int64
	err := h.dbRef.Get().QueryRowContext(ctx,
		`SELECT id FROM gt_knowledge_folders WHERE name=? AND parent_id=?`, name, parentID).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	res, err := h.dbRef.Get().ExecContext(ctx,
		`INSERT INTO gt_knowledge_folders (name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		name, parentID, nowMillis(), nowMillis())
	if err != nil {
		return 0, false, err
	}
	nid, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return nid, true, nil
}

// ==================== Reference resolution (BE-06) ====================

// DocumentPath resolves a document uuid to its KR absolute path (BE-06).
// GET /api/local/knowledge/documents/:uuid/path
func (h *Handler) DocumentPath(c *gin.Context) {
	docUUID := c.Param("uuid")
	if docUUID == "" {
		i18n.Error(c, http.StatusBadRequest, "knowledge_uuid_required", "")
		return
	}
	krDir, err := h.contentDirProvider()
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	var filePath string
	var deleted int
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT file_path, deleted FROM gt_knowledge_documents WHERE uuid=?`, docUUID).
		Scan(&filePath, &deleted)
	if err == sql.ErrNoRows {
		// S-BE-06: 文档不存在 -> 明确返回 exists=false（验收②）
		c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "absolute_path": "", "exists": false})
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}

	exists := deleted == 0
	absolute := ""
	if exists {
		store, serr := NewStore(krDir)
		if serr == nil {
			if full, perr := store.DocPath(filePath); perr == nil {
				if _, serr2 := os.Stat(full); serr2 == nil {
					absolute = full
				} else {
					exists = false
				}
			}
		}
	}
	// S-BE-06: 文档已删/文件缺失返回 exists=false（验收②）
	c.JSON(http.StatusOK, gin.H{"uuid": docUUID, "absolute_path": absolute, "exists": exists})
}

// ==================== Search (BE-05 / DA-05) ====================

// Search full text search over indexed documents' real files (BE-05 验收①).
// GET /api/local/knowledge/search?q=keywords
func (h *Handler) Search(c *gin.Context) {
	store, storeErr := h.getStore(c)
	if storeErr != nil {
		i18n.Error(c, http.StatusUnauthorized, "common_server_error", "")
		return
	}

	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"items": []SearchResult{}, "total": 0})
		return
	}

	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT uuid, folder_id, title, tags_json, file_path
		FROM gt_knowledge_documents WHERE deleted=0 ORDER BY updated_at DESC`)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
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
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		docs = append(docs, d)
	}

	lowerQ := strings.ToLower(q)
	var items []SearchResult

	for _, d := range docs {
		titleMatched := strings.Contains(strings.ToLower(d.title), lowerQ)
		tagsMatched := strings.Contains(strings.ToLower(d.tagsJSON), lowerQ)

		// S-BE-05: 读真实文件内容匹配；>5MB 仅索引不预读
		contentMatched := false
		var content string
		if size, serr := store.FileSize(d.filePath); serr == nil && size <= maxSearchFileBytes {
			if data, rerr := store.ReadFile(d.filePath); rerr == nil {
				content = data
				contentMatched = strings.Contains(strings.ToLower(content), lowerQ)
			}
		}

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

// folderRelPath resolves a folder_id to its real relative directory path (DA-02).
// 返回值统一使用正斜杠，与 file_path 存储规范保持一致。
func (h *Handler) folderRelPath(ctx context.Context, folderID int64) (string, error) {
	folders, err := loadFolders(h.dbRef.Get())
	if err != nil {
		return "", err
	}
	return normalizeRelPath(resolveFolderRel(folders, folderID)), nil
}

// uniqueNewRelPath returns a non-colliding relative path for a new document (DB + disk dedup, DA-02 验收③).
// 注意：数据库中的 file_path 可能含反斜杠，查询时使用规范化后的路径进行比较。
func (h *Handler) uniqueNewRelPath(ctx context.Context, store *Store, relPath string) string {
	normPath := normalizePath(relPath)
	existsDB := func(p string) bool {
		var dummy int
		// 使用规范化后的路径查询，避免 Windows/Unix 路径符不一致
		return h.dbRef.Get().QueryRowContext(ctx,
			`SELECT 1 FROM gt_knowledge_documents WHERE REPLACE(file_path, '\\', '/')=?`, p).Scan(&dummy) == nil
	}
	if !existsDB(normPath) {
		if full, err := store.DocPath(relPath); err == nil {
			if _, serr := os.Stat(full); serr != nil {
				return relPath
			}
		}
	}
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	for n := 1; n < 10000; n++ {
		cand := fmt.Sprintf("%s(%d)%s", base, n, ext)
		normCand := normalizePath(cand)
		if existsDB(normCand) {
			continue
		}
		if full, err := store.DocPath(cand); err == nil {
			if _, serr := os.Stat(full); serr == nil {
				continue
			}
		}
		return cand
	}
	return relPath
}

// moveRelPathUnique returns a non-colliding target path when renaming (DB dedup, excludes self uuid).
// 注意：数据库中的 file_path 可能含反斜杠，查询时使用规范化后的路径进行比较。
func (h *Handler) moveRelPathUnique(ctx context.Context, store *Store, cand, selfUUID string) string {
	normCand := normalizePath(cand)
	collides := func(p string) bool {
		var otherUUID string
		// 使用规范化后的路径查询，避免 Windows/Unix 路径符不一致
		err := h.dbRef.Get().QueryRowContext(ctx,
			`SELECT uuid FROM gt_knowledge_documents WHERE REPLACE(file_path, '\\', '/')=? AND uuid<>?`, p, selfUUID).Scan(&otherUUID)
		return err == nil
	}
	if !collides(normCand) {
		if full, err := store.DocPath(cand); err == nil {
			if _, serr := os.Stat(full); serr != nil {
				return cand
			}
		}
	}
	ext := filepath.Ext(cand)
	base := strings.TrimSuffix(cand, ext)
	for n := 1; n < 10000; n++ {
		next := fmt.Sprintf("%s(%d)%s", base, n, ext)
		normNext := normalizePath(next)
		if collides(normNext) {
			continue
		}
		if full, err := store.DocPath(next); err == nil {
			if _, serr := os.Stat(full); serr == nil {
				continue
			}
		}
		return next
	}
	return cand
}

// collectDescendantFolders returns rootID and all of its descendant folder ids.
func (h *Handler) collectDescendantFolders(ctx context.Context, rootID int64) ([]int64, error) {
	rows, err := h.dbRef.Get().QueryContext(ctx, `SELECT id, parent_id FROM gt_knowledge_folders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	children := make(map[int64][]int64)
	for rows.Next() {
		var id, parentID int64
		if err := rows.Scan(&id, &parentID); err != nil {
			return nil, err
		}
		children[parentID] = append(children[parentID], id)
	}
	result := []int64{}
	visited := make(map[int64]bool)
	stack := []int64{rootID}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		result = append(result, cur)
		stack = append(stack, children[cur]...)
	}
	return result, nil
}

// inClause builds a placeholder list and args for an IN (?) query.
func inClause(ids []int64) (string, []interface{}) {
	ph := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		ph = append(ph, "?")
		args = append(args, id)
	}
	return strings.Join(ph, ","), args
}

// checkDirWritable verifies a directory is creatable and writable (DA-01 验收①).
func checkDirWritable(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".kr_write_test_"+uuid.New().String())
	if err := os.WriteFile(tmp, []byte("ok"), 0644); err != nil {
		return err
	}
	return os.Remove(tmp)
}

// ==================== Utility ====================

// normalizePath 将路径中的反斜杠统一替换为正斜杠，确保跨平台一致性。
// 数据库中的 file_path 统一使用正斜杠存储和比较。
func normalizePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// normalizeRelPath 在路径规范化后确保不以 / 开头（用于拼接存储路径）。
func normalizeRelPath(path string) string {
	result := normalizePath(path)
	result = strings.TrimPrefix(result, "/")
	return result
}

// pathsEqual 比较两个路径是否相等，均先进行规范化处理以避免历史数据中的路径符不一致问题。
func pathsEqual(path1, path2 string) bool {
	return normalizePath(path1) == normalizePath(path2)
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
