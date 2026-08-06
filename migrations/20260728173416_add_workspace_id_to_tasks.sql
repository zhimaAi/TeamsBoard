-- +goose Up
-- +goose StatementBegin

-- 任务主表补充 workspace_id（项目ID），用于同步到云端 agent_tasks.workspace_id
ALTER TABLE gt_tasks ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 0;

-- 回填：从已保存的工作项快照 JSON 中恢复 workspace_id（仅对已有工作项的任务）
UPDATE gt_tasks
SET workspace_id = CAST(
    COALESCE(
        json_extract(
            (SELECT snapshot_json
             FROM gt_work_item_snapshots w
             WHERE w.admin_id = gt_tasks.admin_id
               AND w.user_id = gt_tasks.user_id
               AND w.work_item_type = gt_tasks.work_item_type
               AND w.work_item_id = gt_tasks.work_item_id),
            '$.workspace_id'),
        '0') AS INTEGER)
WHERE work_item_id <> '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE gt_tasks DROP COLUMN workspace_id;

-- +goose StatementEnd
