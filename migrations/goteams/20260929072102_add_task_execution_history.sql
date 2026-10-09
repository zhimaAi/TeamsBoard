-- +goose Up
CREATE TABLE IF NOT EXISTS gt_task_execution_history (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    execution_no INTEGER NOT NULL,
    execution_mode TEXT NOT NULL DEFAULT '',
    execution_tool TEXT NOT NULL DEFAULT '',
    target_name TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL DEFAULT 0,
    archived_at INTEGER NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE(task_uuid, execution_no)
);

CREATE INDEX IF NOT EXISTS idx_gt_task_execution_history_task
    ON gt_task_execution_history(task_uuid, execution_no);
