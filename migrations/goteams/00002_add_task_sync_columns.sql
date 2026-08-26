-- +goose Up
-- +goose StatementBegin
-- 云端任务结果推送需要工作项与工作空间关联；taskruntime 创建任务时已写入这些字段，
-- 这里补齐历史数据库缺失的列（SQLite ALTER TABLE ADD COLUMN 不可重复执行，由 goose 版本表保证只跑一次）。
ALTER TABLE gt_tasks ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_tasks ADD COLUMN work_item_type TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_tasks ADD COLUMN work_item_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE gt_tasks DROP COLUMN workspace_id;
ALTER TABLE gt_tasks DROP COLUMN work_item_type;
ALTER TABLE gt_tasks DROP COLUMN work_item_id;
-- +goose StatementEnd
