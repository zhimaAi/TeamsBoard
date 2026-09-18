-- +goose Up
-- 任务执行方式由应用层校验，按仓库规范不使用 CHECK 约束。
ALTER TABLE gt_tasks ADD COLUMN execution_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_tasks ADD COLUMN execution_tool TEXT NOT NULL DEFAULT '';

UPDATE gt_tasks
SET execution_mode = 'pipeline'
WHERE execution_mode = ''
  AND (
    selected_pipeline_uuid <> ''
    OR pipeline_snapshot_uuid <> ''
    OR EXISTS (SELECT 1 FROM gt_task_steps WHERE gt_task_steps.task_uuid = gt_tasks.uuid)
  );

-- 所有执行方式共用 gt_task_progress。流水线记录保留步骤字段，
-- Vibe Coding 记录仅绑定 task_uuid，并将步骤字段保存为空字符串。
ALTER TABLE gt_task_progress ADD COLUMN execution_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_task_progress ADD COLUMN external_session_id TEXT NOT NULL DEFAULT '';

UPDATE gt_task_progress
SET execution_mode = 'pipeline'
WHERE execution_mode = '';
