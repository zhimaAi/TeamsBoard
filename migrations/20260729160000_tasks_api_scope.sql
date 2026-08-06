-- +goose Up
-- +goose StatementBegin

ALTER TABLE gt_tasks ADD COLUMN api_collection_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_tasks ADD COLUMN api_collection_name_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_tasks ADD COLUMN api_folder_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_tasks ADD COLUMN api_folder_name_snapshot TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_gt_tasks_api_scope
    ON gt_tasks(api_collection_id, api_folder_id);

-- +goose StatementEnd

-- +goose Down
-- SQLite 不安全地移除已有列；回滚只移除索引。
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_gt_tasks_api_scope;
-- +goose StatementEnd
