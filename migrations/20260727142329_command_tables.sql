-- +goose Up
-- +goose StatementBegin

-- 命令执行记录表
CREATE TABLE IF NOT EXISTS gt_command_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command_type TEXT NOT NULL DEFAULT '',      -- git | docker
    project_id INTEGER NOT NULL DEFAULT 0,      -- 关联的项目 ID
    command TEXT NOT NULL DEFAULT '',           -- 执行的子命令
    args_json TEXT NOT NULL DEFAULT '[]',       -- 参数列表 JSON
    work_dir TEXT NOT NULL DEFAULT '',           -- 工作目录
    status TEXT NOT NULL DEFAULT 'running',     -- running | success | failed | stopped
    exit_code INTEGER NOT NULL DEFAULT -1,
    output TEXT NOT NULL DEFAULT '',             -- 合并的输出（截断到 100KB）
    duration_ms INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    finished_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_gt_command_runs_type ON gt_command_runs(command_type, started_at);
CREATE INDEX IF NOT EXISTS idx_gt_command_runs_project ON gt_command_runs(project_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_gt_command_runs_project;
DROP INDEX IF EXISTS idx_gt_command_runs_type;
DROP TABLE IF EXISTS gt_command_runs;
-- +goose StatementEnd
