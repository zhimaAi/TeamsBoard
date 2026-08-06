-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS gt_task_work_dirs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT NOT NULL REFERENCES gt_tasks(uuid),
    path TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    UNIQUE(task_uuid, path)
);

CREATE INDEX IF NOT EXISTS idx_gt_task_work_dirs_task
    ON gt_task_work_dirs(task_uuid, sort_order, id);

-- 兼容已有任务：把原 work_dir 迁移为每个任务的主目录。
INSERT OR IGNORE INTO gt_task_work_dirs (task_uuid, path, sort_order, created_at)
SELECT uuid, work_dir, 0, created_at
FROM gt_tasks
WHERE TRIM(work_dir) <> '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_gt_task_work_dirs_task;
DROP TABLE IF EXISTS gt_task_work_dirs;

-- +goose StatementEnd
