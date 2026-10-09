-- +goose Up
-- 微信通道属于本机任务库，凭据另存于系统 SecretStore。
CREATE TABLE IF NOT EXISTS gt_remote_wechat_state (
    id INTEGER PRIMARY KEY,
    bot_id TEXT NOT NULL DEFAULT '',
    sender_id TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT 'zh-CN',
    base_url TEXT NOT NULL DEFAULT '',
    cursor TEXT NOT NULL DEFAULT '',
    bound_task_uuid TEXT NOT NULL DEFAULT '',
    context_revision INTEGER NOT NULL DEFAULT 0,
    connected_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS gt_remote_task_aliases (
    short_id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_remote_wechat_inbox (
    message_id TEXT PRIMARY KEY,
    text_content TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    task_uuid TEXT NOT NULL DEFAULT '',
    session_uuid TEXT NOT NULL DEFAULT '',
    next_recovery_at INTEGER NOT NULL DEFAULT 0,
    recovery_attempts INTEGER NOT NULL DEFAULT 0,
    received_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gt_remote_wechat_inbox_status ON gt_remote_wechat_inbox(status, received_at);
CREATE INDEX IF NOT EXISTS idx_gt_remote_wechat_inbox_recovery
    ON gt_remote_wechat_inbox(status, next_recovery_at, received_at);

CREATE TABLE IF NOT EXISTS gt_remote_wechat_outbox (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL UNIQUE,
    task_uuid TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    blocked_context_revision INTEGER NOT NULL DEFAULT -1,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    delivered_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gt_remote_wechat_outbox_pending ON gt_remote_wechat_outbox(status, next_attempt_at, created_at);
