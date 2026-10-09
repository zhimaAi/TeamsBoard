package localserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"goteams-client/internal/pipeline"
	"goteams-client/internal/taskruntime"
)

// wechatDispatchFailure carries only a safe category and translation key.
// Raw runtime errors and workspace paths must not be sent to WeChat.
type wechatDispatchFailure struct {
	category string
	stage    string
	key      string
}

func (e *wechatDispatchFailure) Error() string { return e.category + ": " + e.stage }

func wechatConfigFailure(stage, key string) error {
	return &wechatDispatchFailure{category: "configuration", stage: stage, key: key}
}

func wechatRuntimeFailure(stage string) error {
	return &wechatDispatchFailure{category: "runtime", stage: stage, key: "remote_wechat_dispatch_failed"}
}

func (w *wechatRemote) validateDispatchWorkspace(ctx context.Context, taskUUID string) error {
	var workDir, taskDir string
	if err := w.db.QueryRowContext(ctx, `SELECT work_dir, task_dir FROM gt_tasks WHERE uuid=?`, taskUUID).
		Scan(&workDir, &taskDir); err != nil {
		return err
	}
	for _, dir := range []string{workDir, taskDir} {
		if strings.TrimSpace(dir) == "" {
			return wechatConfigFailure("workspace", "remote_wechat_config_workspace")
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return wechatConfigFailure("workspace", "remote_wechat_config_workspace")
		}
	}
	return nil
}

func (w *wechatRemote) validateDispatchStep(ctx context.Context, taskUUID, stepUUID string) error {
	var cliType, model string
	err := w.db.QueryRowContext(ctx, `SELECT cli_type, model_name FROM gt_task_steps WHERE task_uuid=? AND uuid=?`,
		taskUUID, stepUUID).Scan(&cliType, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return wechatConfigFailure("step", "remote_wechat_config_step")
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(cliType) == "" {
		return wechatConfigFailure("cli", "remote_wechat_config_cli")
	}
	if strings.TrimSpace(model) == "" {
		return wechatConfigFailure("model", "remote_wechat_config_model")
	}
	return nil
}

func (w *wechatRemote) validatePipelineSteps(ctx context.Context, taskUUID string) error {
	rows, err := w.db.QueryContext(ctx, `SELECT s.cli_type, s.model_name FROM gt_task_steps s
		JOIN gt_tasks t ON t.uuid=s.task_uuid AND t.pipeline_snapshot_uuid=s.task_pipeline_snapshot_uuid
		WHERE t.uuid=? ORDER BY s.step_order, s.rowid`, taskUUID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var cliType, model string
		if err := rows.Scan(&cliType, &model); err != nil {
			return err
		}
		count++
		if strings.TrimSpace(cliType) == "" {
			return wechatConfigFailure("cli", "remote_wechat_config_cli")
		}
		if strings.TrimSpace(model) == "" {
			return wechatConfigFailure("model", "remote_wechat_config_model")
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count == 0 {
		return wechatConfigFailure("pipeline", "remote_wechat_config_pipeline")
	}
	return nil
}

// preparePipelineMessage freezes a complete saved configuration and activates
// the first step. The task's board status is independent of this operation.
func (w *wechatRemote) preparePipelineMessage(ctx context.Context, task wechatTask) (string, error) {
	if w.tasks == nil || w.orchestrator == nil {
		return "", errors.New("任务执行服务不可用")
	}
	var snapshotUUID, selectedUUID, configJSON string
	if err := w.db.QueryRowContext(ctx, `SELECT pipeline_snapshot_uuid, selected_pipeline_uuid, selected_pipeline_config_json
		FROM gt_tasks WHERE uuid=?`, task.UUID).Scan(&snapshotUUID, &selectedUUID, &configJSON); err != nil {
		return "", err
	}
	if snapshotUUID == "" {
		if strings.TrimSpace(selectedUUID) == "" {
			return "", wechatConfigFailure("pipeline", "remote_wechat_config_pipeline")
		}
		var configs []stepExecutionConfig
		if strings.TrimSpace(configJSON) != "" && json.Unmarshal([]byte(configJSON), &configs) != nil {
			return "", wechatConfigFailure("pipeline", "remote_wechat_config_pipeline")
		}
		snapshot, err := w.tasks.snapshotFromPipeline(ctx, pipelineAssignmentInput{
			PipelineUUID: selectedUUID, StepConfigs: configs,
		})
		if errors.Is(err, pipeline.ErrNotFound) {
			return "", wechatConfigFailure("pipeline", "remote_wechat_config_pipeline")
		}
		if err != nil {
			return "", err
		}
		for _, step := range snapshot.Steps {
			if strings.TrimSpace(step.CLIType) == "" {
				return "", wechatConfigFailure("cli", "remote_wechat_config_cli")
			}
			if strings.TrimSpace(step.ModelName) == "" {
				return "", wechatConfigFailure("model", "remote_wechat_config_model")
			}
		}
		if err := taskruntime.ValidateSnapshotFull(&snapshot); err != nil {
			return "", wechatConfigFailure("pipeline", "remote_wechat_config_pipeline")
		}
		if err := taskruntime.NewService(w.db, w.tasks.taskRoot).AssignPipelineSelection(
			ctx, task.UUID, selectedUUID, configJSON, snapshot); err != nil {
			return "", err
		}
	}
	if err := w.validatePipelineSteps(ctx, task.UUID); err != nil {
		return "", err
	}
	return taskruntime.NewService(w.db, w.tasks.taskRoot).Start(ctx, task.UUID)
}
