package workitem

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"goteams-client/internal/cloud"
	"goteams-client/internal/workflow"
)

// ErrWorkItemAlreadyInTask means the same work item is already occupied by another task
var ErrWorkItemAlreadyInTask = errors.New("work item already in a task")
var ErrInvalidAPIScope = errors.New("invalid task api scope")
var ErrInvalidDBProfile = errors.New("invalid task db profile")

// Service is the work-item and task-creation service
type Service struct {
	cloudClient     *cloud.Client
	store           *Store
	db              *sql.DB
	dataDir         string
	rootDir         string // The data root directory (where skills/runtime live) is passed explicitly to NewService, avoiding multi-level filepath.Dir back-tracking
	localAPIBaseURL string
	skillsRoot      string
}

// ConfigureSkillRuntime sets the local request address and installed Skill root directory to inject into the task prompt.
func (s *Service) ConfigureSkillRuntime(baseURL, skillsRoot string) {
	s.localAPIBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s.skillsRoot = skillsRoot
}

// NewService creates the work-item service.
// dataDir is the account data directory (<root>/accounts/<userID>); rootDir is the data root directory (<root>),
// used to derive the locations of skills and runtime; passed explicitly rather than back-tracked from dataDir.
func NewService(cloudClient *cloud.Client, db *sql.DB, dataDir, rootDir string) *Service {
	return &Service{
		cloudClient: cloudClient,
		store:       NewStore(db),
		db:          db,
		dataDir:     dataDir,
		rootDir:     rootDir,
	}
}

// FetchFromCloud fetches work items from the cloud
func (s *Service) FetchFromCloud(ctx context.Context) ([]cloud.WorkItem, error) {
	return s.cloudClient.GetWorkItems(ctx)
}

// FilterExisting filters out work items already present locally
func (s *Service) FilterExisting(adminID, userID string, items []cloud.WorkItem) ([]cloud.WorkItem, error) {
	var result []cloud.WorkItem
	for _, item := range items {
		exists, err := s.store.Exists(adminID, userID, item.Type, fmt.Sprintf("%d", item.ID))
		if err != nil {
			return nil, err
		}
		if !exists {
			result = append(result, item)
		}
	}
	return result, nil
}

// CreateTaskOptions are the task-creation options
type CreateTaskOptions struct {
	AdminID         string
	UserID          string
	WorkItem        cloud.WorkItem
	AgentSnapshot   *cloud.AgentSnapshot
	WorkDir         string
	WorkDirs        []string
	APICollectionID int64
	APIFolderID     int64
	DBProfileIDs    []int64
	Status          string
}

// CreateTask creates a task (local transaction)
func (s *Service) CreateTask(ctx context.Context, opts CreateTaskOptions) (string, error) {
	workDirs := opts.WorkDirs
	if len(workDirs) == 0 && strings.TrimSpace(opts.WorkDir) != "" {
		workDirs = []string{opts.WorkDir}
	}
	if len(workDirs) == 0 {
		return "", fmt.Errorf("工作目录不能为空")
	}
	seenWorkDirs := make(map[string]struct{}, len(workDirs))
	for index, workDir := range workDirs {
		workDir = strings.TrimSpace(workDir)
		if workDir == "" {
			return "", fmt.Errorf("第 %d 个工作目录不能为空", index+1)
		}
		// Normalize paths: resolve to absolute and clean; on Windows, lowercase uniformly for case-insensitive dedup,
		// consistent with localserver's normalizeExistingDirectories.
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			return "", fmt.Errorf("第 %d 个工作目录解析失败: %w", index+1, err)
		}
		absWorkDir = filepath.Clean(absWorkDir)
		key := absWorkDir
		if runtime.GOOS == "windows" {
			key = strings.ToLower(absWorkDir)
		}
		if _, exists := seenWorkDirs[key]; exists {
			return "", fmt.Errorf("工作目录不能重复: %s", absWorkDir)
		}
		seenWorkDirs[key] = struct{}{}
		workDirInfo, err := os.Stat(absWorkDir)
		if err != nil {
			return "", fmt.Errorf("工作目录不可用: %w", err)
		}
		if !workDirInfo.IsDir() {
			return "", fmt.Errorf("工作目录不是文件夹: %s", absWorkDir)
		}
		workDirs[index] = absWorkDir
	}
	primaryWorkDir := workDirs[0]

	// The same requirement/defect can belong to only one task: check whether it is already occupied by another task
	var existingTitle, existingUUID string
	err := s.db.QueryRow(
		`SELECT uuid, title FROM gt_tasks WHERE admin_id = ? AND user_id = ? AND work_item_type = ? AND work_item_id = ? LIMIT 1`,
		opts.AdminID, opts.UserID, opts.WorkItem.Type, fmt.Sprintf("%d", opts.WorkItem.ID),
	).Scan(&existingUUID, &existingTitle)
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("检查工作项占用失败: %w", err)
	}
	if existingUUID != "" {
		typeLabel := "需求"
		if opts.WorkItem.Type == "defect" {
			typeLabel = "缺陷"
		}
		return "", fmt.Errorf("%w: 该%s「%s」已存在于任务「%s」中，不能重复创建",
			ErrWorkItemAlreadyInTask, typeLabel, opts.WorkItem.Title, existingTitle)
	}

	taskUUID := uuid.New().String()
	now := time.Now().UnixMilli()

	// Initial task status: defaults to 'pending' when the caller does not specify
	status := opts.Status
	if status == "" {
		status = "pending"
	}

	// Serialize the work-item snapshot
	workItemJSON, _ := json.Marshal(opts.WorkItem)

	// Compute workflow_snapshot_hash
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%d:%s:%d", opts.AgentSnapshot.ID, opts.AgentSnapshot.Name, opts.AgentSnapshot.WorkflowVersion)))
	for _, step := range opts.AgentSnapshot.Steps {
		hasher.Write([]byte(step.StepKey))
		hasher.Write([]byte(step.Prompt))
	}
	workflowHash := hex.EncodeToString(hasher.Sum(nil))

	// Task title
	title := opts.WorkItem.Title
	if title == "" {
		title = fmt.Sprintf("%s #%d", opts.WorkItem.Type, opts.WorkItem.ID)
	}

	// Begin transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()
	committed := false
	var createdDocumentFiles []string
	defer func() {
		if committed {
			return
		}
		for _, path := range createdDocumentFiles {
			_ = os.Remove(path)
		}
	}()

	apiCollectionName, apiFolderID, apiFolderName, err := ensureTaskAPIScope(
		ctx, tx, opts.APICollectionID, opts.APIFolderID, title, now,
	)
	if err != nil {
		return "", err
	}

	// Parse and validate the task's selected database configs (optional, multiple allowed)
	dbProfiles, err := ensureTaskDBProfiles(ctx, tx, opts.DBProfileIDs)
	if err != nil {
		return "", err
	}
	// Write the task-database config association
	for index, profile := range dbProfiles {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO gt_task_db_profiles (task_uuid, database_profile_id, sort_order) VALUES (?, ?, ?)`,
			taskUUID, profile.ID, index,
		); err != nil {
			return "", fmt.Errorf("保存任务数据库配置失败: %w", err)
		}
	}

	// 1. Write the work-item snapshot
	_, err = tx.Exec(
		`INSERT INTO gt_work_item_snapshots (admin_id, user_id, work_item_type, work_item_id, snapshot_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(admin_id, user_id, work_item_type, work_item_id) DO UPDATE SET snapshot_json = excluded.snapshot_json`,
		opts.AdminID, opts.UserID, opts.WorkItem.Type, fmt.Sprintf("%d", opts.WorkItem.ID), string(workItemJSON), now)
	if err != nil {
		return "", fmt.Errorf("写入工作项快照失败: %w", err)
	}

	// 2. Insert the task
	_, err = tx.Exec(
		`INSERT INTO gt_tasks (uuid, admin_id, user_id, agent_id, agent_name_snapshot, agent_color_snapshot, cli_type, workflow_version, work_item_type, work_item_id, workspace_id, title, content_snapshot, work_dir,
		 api_collection_id, api_collection_name_snapshot, api_folder_id, api_folder_name_snapshot,
		 status, execution_status, workflow_snapshot_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'idle', ?, ?, ?)`,
		taskUUID, opts.AdminID, opts.UserID,
		fmt.Sprintf("%d", opts.AgentSnapshot.ID),
		opts.AgentSnapshot.Name, opts.AgentSnapshot.Color, opts.AgentSnapshot.CLIType, opts.AgentSnapshot.WorkflowVersion,
		opts.WorkItem.Type, fmt.Sprintf("%d", opts.WorkItem.ID), opts.WorkItem.WorkspaceID,
		title, opts.WorkItem.Description, primaryWorkDir,
		opts.APICollectionID, apiCollectionName, apiFolderID, apiFolderName,
		status, workflowHash, now, now)
	if err != nil {
		return "", fmt.Errorf("插入任务失败: %w", err)
	}

	// 3. Save the task's primary and related working directories.
	for index, workDir := range workDirs {
		if _, err = tx.Exec(
			`INSERT INTO gt_task_work_dirs (task_uuid, path, sort_order, created_at)
			 VALUES (?, ?, ?, ?)`,
			taskUUID, workDir, index, now,
		); err != nil {
			return "", fmt.Errorf("保存任务工作目录失败: %w", err)
		}
	}

	// 4. Materialize the custom documents actually referenced by the requirement and steps into the local knowledge base.
	requirementPath, customDocumentPaths, documentFiles, err := s.createTaskDocuments(
		ctx, tx, taskUUID, opts.WorkItem, opts.AgentSnapshot, now,
	)
	createdDocumentFiles = append(createdDocumentFiles, documentFiles...)
	if err != nil {
		return "", err
	}

	rootDir := s.rootDir
	skillsRoot := s.skillsRoot
	if skillsRoot == "" {
		skillsRoot = filepath.Join(rootDir, "skills")
	}
	skillsAPIPath := filepath.Join(skillsRoot, "goteams-api")
	skillsDBPath := filepath.Join(skillsRoot, "goteams-db")

	// 5. Insert task steps
	for _, step := range opts.AgentSnapshot.Steps {
		stepUUID := uuid.New().String()
		stepDocumentsJSON, marshalErr := json.Marshal(step.Documents)
		if marshalErr != nil {
			return "", fmt.Errorf("序列化步骤文档快照失败: %w", marshalErr)
		}
		placeholderCtx := workflow.PlaceholderContext{
			RequirementDocPath: requirementPath,
			DocumentPaths:      customDocumentPaths,
			SkillsAPIPath:      skillsAPIPath,
			SkillsDBPath:       skillsDBPath,
		}
		// Built-in placeholder registry: sent by cloud (key -> token), strictly cloud-dependent with no fallback
		placeholderCtx.BuiltinPlaceholders = make(map[string]string, len(opts.AgentSnapshot.BuiltinPlaceholders))
		for _, bp := range opts.AgentSnapshot.BuiltinPlaceholders {
			if bp.Key != "" && bp.Token != "" {
				placeholderCtx.BuiltinPlaceholders[bp.Key] = bp.Token
			}
		}
		promptSnapshot := workflow.ResolveStatic(step.Prompt, placeholderCtx)
		promptSnapshot = workflow.InjectSkillPromptContext(promptSnapshot, step.Prompt, workflow.SkillPromptContext{
			APIBaseURL:        s.localAPIBaseURL,
			SkillsRoot:        skillsRoot,
			APICollectionID:   opts.APICollectionID,
			APICollectionName: apiCollectionName,
			APIFolderID:       apiFolderID,
			APIFolderName:     apiFolderName,
			DBProfiles:        dbProfiles,
		})
		promptSnapshot = workflow.InjectWorkDirPromptContext(promptSnapshot, workDirs)
		_, err = tx.Exec(
			`INSERT INTO gt_task_steps (uuid, task_uuid, step_key, step_order, name, cli_type, prompt_snapshot, documents_snapshot_json, status, execution_status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'idle', ?, ?)`,
			stepUUID, taskUUID, step.StepKey, step.SortOrder, step.Name, opts.AgentSnapshot.CLIType, promptSnapshot, string(stepDocumentsJSON), now, now)
		if err != nil {
			return "", fmt.Errorf("插入步骤失败: %w", err)
		}
	}

	// 6. Insert sync status
	_, err = tx.Exec(
		`INSERT INTO gt_task_sync_state (task_uuid, local_revision, projection_hash, sync_dirty, pending_event_id, cloud_task_id, last_uploaded_at, updated_at)
		 VALUES (?, 1, '', 1, '', '', 0, ?)`,
		taskUUID, now)
	if err != nil {
		return "", fmt.Errorf("插入同步状态失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("提交事务失败: %w", err)
	}
	committed = true

	// 7. Create the task run directory.
	s.materializeTaskFiles(taskUUID)

	return taskUUID, nil
}

func ensureTaskAPIScope(
	ctx context.Context,
	tx *sql.Tx,
	collectionID, folderID int64,
	taskTitle string,
	now int64,
) (string, int64, string, error) {
	// The API collection is optional: when not selected, the task binds no API scope
	if collectionID <= 0 {
		return "", 0, "", nil
	}
	if folderID < 0 {
		return "", 0, "", fmt.Errorf("%w: 接口文件夹 ID 无效", ErrInvalidAPIScope)
	}
	var collectionName string
	if err := tx.QueryRowContext(ctx,
		`SELECT name FROM gt_api_collections WHERE id = ?`, collectionID,
	).Scan(&collectionName); err == sql.ErrNoRows {
		return "", 0, "", fmt.Errorf("%w: 接口集合不存在", ErrInvalidAPIScope)
	} else if err != nil {
		return "", 0, "", fmt.Errorf("查询接口集合失败: %w", err)
	}

	if folderID > 0 {
		var folderName string
		if err := tx.QueryRowContext(ctx,
			`SELECT name FROM gt_api_folders WHERE id = ? AND collection_id = ?`, folderID, collectionID,
		).Scan(&folderName); err == sql.ErrNoRows {
			return "", 0, "", fmt.Errorf("%w: 接口文件夹不属于所选集合", ErrInvalidAPIScope)
		} else if err != nil {
			return "", 0, "", fmt.Errorf("查询接口文件夹失败: %w", err)
		}
		return collectionName, folderID, folderName, nil
	}

	baseName := truncateFolderName(taskTitle, 80)
	for suffix := 1; suffix <= 999; suffix++ {
		name := baseName
		if suffix > 1 {
			tail := fmt.Sprintf("-%d", suffix)
			name = truncateFolderName(baseName, 80-len([]rune(tail))) + tail
		}
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM gt_api_folders WHERE collection_id = ? AND parent_id = 0 AND name = ?`,
			collectionID, name,
		).Scan(&count); err != nil {
			return "", 0, "", fmt.Errorf("检查接口文件夹名称失败: %w", err)
		}
		if count > 0 {
			continue
		}
		result, err := tx.ExecContext(ctx,
			`INSERT INTO gt_api_folders
			 (collection_id, parent_id, name, description, sort_order, created_at, updated_at)
			 VALUES (?, 0, ?, ?, 0, ?, ?)`,
			collectionID, name, "由任务「"+taskTitle+"」自动创建", now, now,
		)
		if err != nil {
			return "", 0, "", fmt.Errorf("自动创建接口文件夹失败: %w", err)
		}
		createdID, err := result.LastInsertId()
		if err != nil {
			return "", 0, "", fmt.Errorf("读取接口文件夹 ID 失败: %w", err)
		}
		return collectionName, createdID, name, nil
	}
	return "", 0, "", fmt.Errorf("%w: 无法为任务生成唯一接口文件夹", ErrInvalidAPIScope)
}

func ensureTaskDBProfiles(
	ctx context.Context,
	tx *sql.Tx,
	dbProfileIDs []int64,
) ([]workflow.DBProfileRef, error) {
	refs := make([]workflow.DBProfileRef, 0, len(dbProfileIDs))
	seen := make(map[int64]struct{}, len(dbProfileIDs))
	for _, id := range dbProfileIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		var name, dbType, databaseName string
		if err := tx.QueryRowContext(ctx,
			`SELECT name, db_type, database_name FROM gt_database_profiles WHERE id = ?`, id,
		).Scan(&name, &dbType, &databaseName); err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: 数据库连接 %d 不存在", ErrInvalidDBProfile, id)
		} else if err != nil {
			return nil, fmt.Errorf("查询数据库配置失败: %w", err)
		}
		refs = append(refs, workflow.DBProfileRef{
			ID:           id,
			Name:         name,
			DBType:       dbType,
			DatabaseName: databaseName,
		})
	}
	return refs, nil
}

func truncateFolderName(value string, maxRunes int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		value = "未命名任务"
	}
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		return strings.TrimSpace(string(runes[:maxRunes]))
	}
	return value
}

// createTaskDocuments creates knowledge-base documents and task associations, returning absolute paths usable for placeholder replacement.
func (s *Service) createTaskDocuments(
	ctx context.Context,
	tx *sql.Tx,
	taskUUID string,
	workItem cloud.WorkItem,
	agentSnapshot *cloud.AgentSnapshot,
	now int64,
) (string, map[string]string, []string, error) {
	contentDir := filepath.Join(s.dataDir, "knowledge", "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return "", nil, nil, fmt.Errorf("创建知识库目录失败: %w", err)
	}

	stepDocumentCount := 0
	for _, step := range agentSnapshot.Steps {
		stepDocumentCount += len(step.Documents)
	}
	createdFiles := make([]string, 0, 1+len(agentSnapshot.Documents)+stepDocumentCount)
	createDocument := func(documentType, clientDocumentID, name, content string) (string, error) {
		documentUUID := uuid.New().String()
		documentPath, err := filepath.Abs(filepath.Join(contentDir, documentUUID+".md"))
		if err != nil {
			return "", fmt.Errorf("解析知识库文档路径失败: %w", err)
		}
		if err := os.WriteFile(documentPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("写入知识库文档失败: %w", err)
		}
		createdFiles = append(createdFiles, documentPath)

		hashValue := knowledgeContentHash(content)
		wordCount := knowledgeWordCount(content)
		tagsJSON, _ := json.Marshal([]string{"task-document", documentType})
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO gt_knowledge_documents
			 (uuid, folder_id, title, file_path, tags_json, content_hash, word_count, created_at, updated_at, deleted)
			 VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, 0)`,
			documentUUID, name, filepath.Base(documentPath), string(tagsJSON), hashValue, wordCount, now, now,
		); err != nil {
			return "", fmt.Errorf("写入知识库文档索引失败: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO gt_task_documents
			 (task_uuid, document_type, client_document_id, knowledge_document_uuid, document_name, document_path, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			taskUUID, documentType, clientDocumentID, documentUUID, name, documentPath, now,
		); err != nil {
			return "", fmt.Errorf("写入任务文档关联失败: %w", err)
		}
		return documentPath, nil
	}

	requirementName := workItem.Title
	if requirementName == "" {
		requirementName = fmt.Sprintf("需求 #%d", workItem.ID)
	}
	requirementPath, err := createDocument(
		"requirement", "", requirementName, workItem.Description,
	)
	if err != nil {
		return "", nil, createdFiles, err
	}

	customPaths := make(map[string]string)
	documentKey := func(document cloud.AgentDocument) string {
		for _, value := range []string{document.ClientDocumentID, document.ID, document.Key, document.Name} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	type plannedDocument struct {
		document cloud.AgentDocument
		required bool
	}
	planned := make([]plannedDocument, 0, len(agentSnapshot.Documents)+stepDocumentCount)
	for _, document := range agentSnapshot.Documents {
		planned = append(planned, plannedDocument{document: document})
	}
	for _, step := range agentSnapshot.Steps {
		for _, document := range step.Documents {
			planned = append(planned, plannedDocument{document: document, required: true})
		}
	}

	createdDocumentPaths := make(map[string]string)
	for _, item := range planned {
		document := item.document
		key := documentKey(document)
		if key == "" || strings.TrimSpace(document.Name) == "" {
			continue
		}
		if existingPath := createdDocumentPaths[key]; existingPath != "" {
			customPaths[document.Name] = existingPath
			continue
		}
		if !item.required {
			placeholder := "{" + document.Name + "}"
			used := false
			for _, step := range agentSnapshot.Steps {
				if strings.Contains(step.Prompt, placeholder) {
					used = true
					break
				}
			}
			if !used {
				continue
			}
		}
		path, err := createDocument(
			"custom", key, document.Name, document.DefaultContent,
		)
		if err != nil {
			return "", nil, createdFiles, err
		}
		createdDocumentPaths[key] = path
		customPaths[document.Name] = path
		if placeholderName := strings.Trim(strings.TrimSpace(document.Placeholder), "{}"); placeholderName != "" {
			customPaths[placeholderName] = path
		}
	}
	return requirementPath, customPaths, createdFiles, nil
}

func knowledgeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])[:16]
}

func knowledgeWordCount(content string) int {
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
		default:
			inWord = false
		}
	}
	return count
}

// materializeTaskFiles materializes task files
func (s *Service) materializeTaskFiles(taskUUID string) {
	rootDir := s.rootDir
	taskDir := filepath.Join(rootDir, "runtime", "tasks", taskUUID)
	if err := os.MkdirAll(taskDir, 0700); err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Join(taskDir, "documents"), 0755)
}
