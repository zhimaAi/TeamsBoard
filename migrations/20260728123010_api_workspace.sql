-- +goose Up
-- +goose StatementBegin

-- 接口目录。首期只在单个集合内使用一层目录，parent_id 为后续嵌套预留。
CREATE TABLE IF NOT EXISTS gt_api_folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection_id INTEGER NOT NULL,
    parent_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(collection_id, parent_id, name)
);

CREATE INDEX IF NOT EXISTS idx_gt_api_folders_collection
    ON gt_api_folders(collection_id, parent_id, sort_order, id);

ALTER TABLE gt_api_requests ADD COLUMN folder_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_api_requests ADD COLUMN query_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE gt_api_requests ADD COLUMN auth_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE gt_api_requests ADD COLUMN body_type TEXT NOT NULL DEFAULT 'none';
ALTER TABLE gt_api_requests ADD COLUMN body_form_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE gt_api_requests ADD COLUMN environment_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_api_requests ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_gt_api_requests_folder
    ON gt_api_requests(collection_id, folder_id, sort_order, id);

-- 重建环境表，移除旧版全局 UNIQUE(name)，改为集合内名称唯一。
CREATE TABLE gt_api_environments_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    variables_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(collection_id, name)
);

INSERT INTO gt_api_environments_v2
    (id, collection_id, name, description, variables_json, created_at, updated_at)
SELECT id, 0, name, '', variables_json, created_at, updated_at
FROM gt_api_environments;

DROP TABLE gt_api_environments;
ALTER TABLE gt_api_environments_v2 RENAME TO gt_api_environments;

CREATE INDEX IF NOT EXISTS idx_gt_api_environments_collection
    ON gt_api_environments(collection_id, id);

ALTER TABLE gt_api_runs ADD COLUMN collection_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_api_runs ADD COLUMN env_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_api_runs ADD COLUMN response_headers_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE gt_api_runs ADD COLUMN response_body_truncated INTEGER NOT NULL DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- SQLite 不安全地移除已有列；回滚只移除本迁移新建的目录表和索引。
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_gt_api_requests_folder;
DROP INDEX IF EXISTS idx_gt_api_folders_collection;
DROP TABLE IF EXISTS gt_api_folders;
-- +goose StatementEnd
