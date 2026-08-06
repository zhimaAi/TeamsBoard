-- +goose Up
-- +goose StatementBegin

-- 任务主表
CREATE TABLE IF NOT EXISTS gt_tasks (
    uuid TEXT PRIMARY KEY,
    admin_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_name_snapshot TEXT NOT NULL DEFAULT '',
    agent_color_snapshot TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    workflow_version INTEGER NOT NULL DEFAULT 0,
    work_item_type TEXT NOT NULL DEFAULT '',
    work_item_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content_snapshot TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    execution_status TEXT NOT NULL DEFAULT 'idle',
    current_step_key TEXT NOT NULL DEFAULT '',
    execution_summary TEXT NOT NULL DEFAULT '',
    workflow_snapshot_hash TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    conversation_rounds INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_tasks_admin_user ON gt_tasks(admin_id, user_id);
CREATE INDEX IF NOT EXISTS idx_gt_tasks_status ON gt_tasks(status);

-- 任务步骤表
CREATE TABLE IF NOT EXISTS gt_task_steps (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL REFERENCES gt_tasks(uuid),
    step_key TEXT NOT NULL,
    step_order INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    prompt_snapshot TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    execution_status TEXT NOT NULL DEFAULT 'idle',
    output_summary TEXT NOT NULL DEFAULT '',
    error_summary TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    conversation_rounds INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_task_steps_task ON gt_task_steps(task_uuid);

-- 任务同步状态表
CREATE TABLE IF NOT EXISTS gt_task_sync_state (
    task_uuid TEXT PRIMARY KEY REFERENCES gt_tasks(uuid),
    local_revision INTEGER NOT NULL DEFAULT 1,
    projection_hash TEXT NOT NULL DEFAULT '',
    sync_dirty INTEGER NOT NULL DEFAULT 1,
    pending_event_id TEXT NOT NULL DEFAULT '',
    cloud_task_id TEXT NOT NULL DEFAULT '',
    last_uploaded_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

-- 工作项快照表
CREATE TABLE IF NOT EXISTS gt_work_item_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    work_item_type TEXT NOT NULL,
    work_item_id TEXT NOT NULL,
    snapshot_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    UNIQUE(admin_id, user_id, work_item_type, work_item_id)
);

-- CLI 会话表
CREATE TABLE IF NOT EXISTS gt_cli_sessions (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL REFERENCES gt_tasks(uuid),
    step_uuid TEXT NOT NULL,
    conversation_uuid TEXT NOT NULL,
    parent_session_uuid TEXT NOT NULL DEFAULT '',
    run_no INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'created',
    cli_type TEXT NOT NULL DEFAULT '',
    external_session_id TEXT NOT NULL DEFAULT '',
    work_dir TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    prompt_snapshot TEXT NOT NULL DEFAULT '',
    prompt_hash TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    stop_requested_at INTEGER NOT NULL DEFAULT 0,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_task ON gt_cli_sessions(task_uuid);
CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_conversation ON gt_cli_sessions(conversation_uuid);

-- 设备注册表
CREATE TABLE IF NOT EXISTS gt_device_registrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id TEXT NOT NULL,
    device_uuid TEXT NOT NULL,
    credential_ref TEXT NOT NULL DEFAULT '',
    registered_at INTEGER NOT NULL,
    UNIQUE(admin_id, device_uuid)
);

-- CLI 事件日志表（合并到主库）
CREATE TABLE IF NOT EXISTS gt_session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    UNIQUE(session_uuid, sequence)
);

CREATE INDEX IF NOT EXISTS idx_gt_session_events_session ON gt_session_events(session_uuid, sequence);

-- 命令输出日志表（合并到主库）
CREATE TABLE IF NOT EXISTS gt_command_output_chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_uuid TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    stream_type TEXT NOT NULL DEFAULT 'stdout',
    content TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    UNIQUE(run_uuid, sequence)
);

-- 接口执行历史表（合并到主库）
CREATE TABLE IF NOT EXISTS gt_api_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_uuid TEXT NOT NULL,
    api_id INTEGER NOT NULL DEFAULT 0,
    method TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    request_headers_json TEXT NOT NULL DEFAULT '{}',
    request_body_preview TEXT NOT NULL DEFAULT '',
    response_status INTEGER NOT NULL DEFAULT 0,
    response_body TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    UNIQUE(run_uuid)
);

CREATE INDEX IF NOT EXISTS idx_gt_api_runs_api_time ON gt_api_runs(api_id, started_at);

-- 会话摘要表
CREATE TABLE IF NOT EXISTS gt_conversation_summaries (
    uuid TEXT PRIMARY KEY,
    conversation_uuid TEXT NOT NULL,
    session_uuid TEXT NOT NULL,
    step_key TEXT NOT NULL DEFAULT '',
    round_no INTEGER NOT NULL DEFAULT 0,
    request_summary TEXT NOT NULL DEFAULT '',
    response_summary TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    token_input INTEGER NOT NULL DEFAULT 0,
    token_output INTEGER NOT NULL DEFAULT 0,
    token_total INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_conversation_summaries_conv ON gt_conversation_summaries(conversation_uuid);

-- 登录账号表
CREATE TABLE IF NOT EXISTS gt_login_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    cloud_base_url TEXT NOT NULL DEFAULT '',
    tenant_name TEXT NOT NULL DEFAULT '',
    user_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    token_secret_ref TEXT NOT NULL DEFAULT '',
    token_expires_at INTEGER NOT NULL DEFAULT 0,
    last_login_at INTEGER NOT NULL DEFAULT 0,
    last_seen_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(admin_id, user_id)
);

-- Agent 缓存表
CREATE TABLE IF NOT EXISTS gt_agent_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    agent_id INTEGER NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    workflow_version INTEGER NOT NULL DEFAULT 0,
    snapshot_json TEXT NOT NULL DEFAULT '',
    fetched_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(admin_id, user_id, agent_id)
);

-- 通用设置表
CREATE TABLE IF NOT EXISTS gt_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scope_type TEXT NOT NULL DEFAULT 'device',
    scope_key TEXT NOT NULL DEFAULT '',
    setting_key TEXT NOT NULL,
    setting_value TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(scope_type, scope_key, setting_key)
);

-- 工具设置表
CREATE TABLE IF NOT EXISTS gt_tool_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tool_key TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    installed_version TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(tool_key)
);

-- 审计日志表
CREATE TABLE IF NOT EXISTS gt_audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    detail_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_audit_events_type ON gt_audit_events(event_type, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gt_audit_events;
DROP TABLE IF EXISTS gt_tool_settings;
DROP TABLE IF EXISTS gt_settings;
DROP TABLE IF EXISTS gt_agent_cache;
DROP TABLE IF EXISTS gt_login_accounts;
DROP TABLE IF EXISTS gt_conversation_summaries;
DROP TABLE IF EXISTS gt_api_runs;
DROP TABLE IF EXISTS gt_command_output_chunks;
DROP TABLE IF EXISTS gt_session_events;
DROP TABLE IF EXISTS gt_device_registrations;
DROP TABLE IF EXISTS gt_cli_sessions;
DROP TABLE IF EXISTS gt_work_item_snapshots;
DROP TABLE IF EXISTS gt_task_sync_state;
DROP TABLE IF EXISTS gt_task_steps;
DROP TABLE IF EXISTS gt_tasks;
-- +goose StatementEnd
