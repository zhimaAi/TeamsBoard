package workitem

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"goteams-client/internal/cloud"
	"goteams-client/internal/workflow"
)

// ErrWorkItemAlreadyInTask means the same work item is already occupied by another task.
var ErrWorkItemAlreadyInTask = errors.New("work item already in task")

// Service is the work-item and task-creation service.
type Service struct {
	cloudClient *cloud.Client
	db          *sql.DB
}

// NewService creates the work-item service.
func NewService(cloudClient *cloud.Client, db *sql.DB) *Service {
	return &Service{
		cloudClient: cloudClient,
		db:          db,
	}
}

// FetchFromCloud fetches work items from the cloud.
func (s *Service) FetchFromCloud(ctx context.Context) ([]cloud.WorkItem, error) {
	return s.cloudClient.GetWorkItems(ctx)
}

// CreateTaskOptions are the task-creation options.
type CreateTaskOptions struct {
	AdminID          string
	UserID           string
	WorkItem         cloud.WorkItem
	PipelineSnapshot *cloud.PipelineSnapshot
	WorkDir          string
	WorkDirs         []string
	Status           string
}

// CreateTask creates a task in a local transaction.
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

	var existingTitle, existingUUID string
	err := s.db.QueryRow(
		`SELECT uuid, title FROM gt_tasks WHERE cloud_admin_id = ? AND cloud_user_id = ? AND cloud_work_item_type = ? AND cloud_work_item_id = ? LIMIT 1`,
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
	status := opts.Status
	if status == "" {
		status = "pending"
	}

	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%d:%s", opts.PipelineSnapshot.ID, opts.PipelineSnapshot.Name)))
	for _, step := range opts.PipelineSnapshot.Steps {
		hasher.Write([]byte(step.StepKey))
		hasher.Write([]byte(step.Prompt))
	}
	workflowHash := hex.EncodeToString(hasher.Sum(nil))

	title := opts.WorkItem.Title
	if title == "" {
		title = fmt.Sprintf("%s #%d", opts.WorkItem.Type, opts.WorkItem.ID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO gt_tasks (uuid, cloud_admin_id, cloud_user_id, cloud_agent_id, cloud_agent_name_snapshot, cloud_agent_color_snapshot, cli_type, cloud_work_item_type, cloud_work_item_id, title, content_snapshot, work_dir,
		 status, execution_status, execution_mode, cloud_workflow_snapshot_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'idle', 'pipeline', ?, ?, ?)`,
		taskUUID, opts.AdminID, opts.UserID,
		fmt.Sprintf("%d", opts.PipelineSnapshot.ID),
		opts.PipelineSnapshot.Name, opts.PipelineSnapshot.Color, opts.PipelineSnapshot.CLIType,
		opts.WorkItem.Type, fmt.Sprintf("%d", opts.WorkItem.ID),
		title, opts.WorkItem.Description, primaryWorkDir,
		status, workflowHash, now, now)
	if err != nil {
		return "", fmt.Errorf("插入任务失败: %w", err)
	}

	for index, workDir := range workDirs {
		if _, err = tx.Exec(
			`INSERT INTO gt_task_work_dirs (task_uuid, path, sort_order, created_at)
			 VALUES (?, ?, ?, ?)`,
			taskUUID, workDir, index, now,
		); err != nil {
			return "", fmt.Errorf("保存任务工作目录失败: %w", err)
		}
	}

	for _, step := range opts.PipelineSnapshot.Steps {
		stepUUID := uuid.New().String()
		promptSnapshot := workflow.InjectWorkDirPromptContext(step.Prompt, workDirs)
		_, err = tx.Exec(
			`INSERT INTO gt_task_steps (uuid, task_uuid, step_key, step_order, name, cli_type, prompt_snapshot, status, execution_status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', 'idle', ?, ?)`,
			stepUUID, taskUUID, step.StepKey, step.SortOrder, step.Name, opts.PipelineSnapshot.CLIType, promptSnapshot, now, now)
		if err != nil {
			return "", fmt.Errorf("插入步骤失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("提交事务失败: %w", err)
	}
	return taskUUID, nil
}
