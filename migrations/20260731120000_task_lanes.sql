-- +goose Up
-- +goose StatementBegin

-- 任务状态（看板列）配置表
CREATE TABLE IF NOT EXISTS gt_task_lanes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lane_key TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '#8c8c8c',
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_hidden INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT 0
);

-- 种子默认状态
INSERT OR IGNORE INTO gt_task_lanes (lane_key, title, color, sort_order, is_hidden, created_at) VALUES
    ('active', '活跃中', '#3157e2', 0, 0, strftime('%s','now') * 1000),
    ('pending', '待开始', '#8c8c8c', 1, 0, strftime('%s','now') * 1000),
    ('developing', '开发中', '#fa8c16', 2, 0, strftime('%s','now') * 1000),
    ('developed', '开发完', '#52c41a', 3, 0, strftime('%s','now') * 1000);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS gt_task_lanes;
