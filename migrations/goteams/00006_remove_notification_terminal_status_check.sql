-- +goose Up
-- +goose StatementBegin
-- 删除 gt_task_notifications.terminal_status 的 CHECK 约束：
-- 通知需要支持 running 等非终态值，数据校验统一在应用层处理。
-- SQLite 不支持 ALTER TABLE DROP CONSTRAINT，按官方流程重建表。
CREATE TABLE gt_task_notifications_new (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    task_step_uuid TEXT NOT NULL,
    step_order INTEGER NOT NULL DEFAULT 0,
    progress_uuid TEXT NOT NULL,
    session_uuid TEXT NOT NULL UNIQUE,
    terminal_status TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    is_read INTEGER NOT NULL DEFAULT 0 CHECK(is_read IN (0, 1)),
    is_archived INTEGER NOT NULL DEFAULT 0 CHECK(is_archived IN (0, 1)),
    created_at INTEGER NOT NULL,
    read_at INTEGER NOT NULL DEFAULT 0
);

INSERT INTO gt_task_notifications_new
    (uuid, task_uuid, task_step_uuid, step_order, progress_uuid, session_uuid,
     terminal_status, title, summary, is_read, is_archived, created_at, read_at)
    SELECT uuid, task_uuid, task_step_uuid, step_order, progress_uuid, session_uuid,
           terminal_status, title, summary, is_read, is_archived, created_at, read_at
    FROM gt_task_notifications;

DROP TABLE gt_task_notifications;
ALTER TABLE gt_task_notifications_new RENAME TO gt_task_notifications;
CREATE INDEX IF NOT EXISTS idx_gt_task_notifications_inbox ON gt_task_notifications(is_archived, is_read, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gt_task_notifications_task ON gt_task_notifications(task_uuid, step_order, created_at);
-- +goose StatementEnd
