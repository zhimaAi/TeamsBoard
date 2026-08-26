-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS gt_pipelines (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT 'local' CHECK(source_type IN ('local', 'cloud')),
    cloud_pipeline_id TEXT NOT NULL DEFAULT '',
    cloud_source_key TEXT NOT NULL DEFAULT '',
    cloud_synced_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_pipelines_cloud_pipeline
    ON gt_pipelines(cloud_source_key, cloud_pipeline_id)
    WHERE source_type = 'cloud' AND cloud_pipeline_id <> '';

CREATE TABLE IF NOT EXISTS gt_pipeline_steps (
    uuid TEXT PRIMARY KEY,
    pipeline_uuid TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    cloud_step_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(pipeline_uuid, sort_order)
);
CREATE INDEX IF NOT EXISTS idx_gt_pipeline_steps_pipeline ON gt_pipeline_steps(pipeline_uuid, sort_order);

CREATE TABLE IF NOT EXISTS gt_projects (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    icon TEXT NOT NULL DEFAULT '',
    icon_type TEXT NOT NULL DEFAULT 'folder',
    icon_url TEXT NOT NULL DEFAULT '',
    main_dir TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_projects_main_dir ON gt_projects(main_dir);

CREATE TABLE IF NOT EXISTS gt_tasks (
    uuid TEXT PRIMARY KEY,
    source_type TEXT NOT NULL DEFAULT 'local' CHECK(source_type IN ('local', 'cloud')),
    cloud_source_key TEXT NOT NULL DEFAULT '',
    cloud_user_id TEXT NOT NULL DEFAULT '',
    cloud_work_item_type TEXT NOT NULL DEFAULT '',
    cloud_work_item_id TEXT NOT NULL DEFAULT '',
    project_uuid TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content_snapshot TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT '',
    planned_start_date TEXT NOT NULL DEFAULT '',
    planned_end_date TEXT NOT NULL DEFAULT '',
    task_dir_id TEXT NOT NULL DEFAULT '',
    task_dir TEXT NOT NULL DEFAULT '',
    task_md_path TEXT NOT NULL DEFAULT '',
    selected_pipeline_uuid TEXT NOT NULL DEFAULT '',
    selected_pipeline_config_json TEXT NOT NULL DEFAULT '[]',
    pipeline_snapshot_uuid TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'active', 'done', 'blocked')),
    execution_status TEXT NOT NULL DEFAULT 'idle',
    current_step_uuid TEXT NOT NULL DEFAULT '',
    current_step_completed INTEGER NOT NULL DEFAULT 0 CHECK(current_step_completed IN (0, 1)),
    work_dir TEXT NOT NULL DEFAULT '',
    -- Compatibility fields retained while the existing handler/service code is replaced.
    -- cloud_user_id above supersedes the legacy user_id column (removed).
    cloud_admin_id TEXT NOT NULL DEFAULT '',
    cloud_agent_id TEXT NOT NULL DEFAULT '',
    cloud_agent_name_snapshot TEXT NOT NULL DEFAULT '',
    cloud_agent_color_snapshot TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    current_step_key TEXT NOT NULL DEFAULT '',
    execution_summary TEXT NOT NULL DEFAULT '',
    cloud_workflow_snapshot_hash TEXT NOT NULL DEFAULT '',
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
CREATE INDEX IF NOT EXISTS idx_gt_tasks_status ON gt_tasks(status);
CREATE INDEX IF NOT EXISTS idx_gt_tasks_execution_status ON gt_tasks(execution_status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_tasks_cloud_work_item_uniq
    ON gt_tasks(cloud_source_key, cloud_user_id, cloud_work_item_type, cloud_work_item_id)
    WHERE source_type = 'cloud' AND cloud_work_item_id <> '';

CREATE TABLE IF NOT EXISTS gt_task_project_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT NOT NULL,
    project_uuid TEXT NOT NULL,
    relation_type TEXT NOT NULL CHECK(relation_type IN ('primary', 'subproject')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    UNIQUE(task_uuid, project_uuid)
);
CREATE INDEX IF NOT EXISTS idx_gt_task_project_links_task
    ON gt_task_project_links(task_uuid, relation_type, sort_order, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_task_project_links_primary
    ON gt_task_project_links(task_uuid) WHERE relation_type = 'primary';

CREATE TABLE IF NOT EXISTS gt_task_pipeline_snapshots (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL UNIQUE,
    source_type TEXT NOT NULL CHECK(source_type IN ('local', 'cloud')),
    source_pipeline_id TEXT NOT NULL DEFAULT '',
    cloud_source_key TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    snapshot_hash TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_task_steps (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    task_pipeline_snapshot_uuid TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT 'local' CHECK(source_type IN ('local', 'cloud')),
    source_step_id TEXT NOT NULL DEFAULT '',
    local_step_uuid TEXT NOT NULL DEFAULT '',
    cloud_step_id TEXT NOT NULL DEFAULT '',
    step_key TEXT NOT NULL,
    step_order INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    prompt_snapshot TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    step_dir TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'active', 'completed')),
    execution_status TEXT NOT NULL DEFAULT 'idle',
    output_summary TEXT NOT NULL DEFAULT '',
    error_summary TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    conversation_rounds INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL DEFAULT 0,
    completed_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(task_uuid, step_key),
    UNIQUE(task_uuid, step_order)
);
CREATE INDEX IF NOT EXISTS idx_gt_task_steps_task ON gt_task_steps(task_uuid, step_order);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_task_steps_one_active ON gt_task_steps(task_uuid) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS gt_task_work_dirs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT NOT NULL,
    path TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    UNIQUE(task_uuid, path),
    UNIQUE(task_uuid, sort_order)
);
CREATE INDEX IF NOT EXISTS idx_gt_task_work_dirs_task ON gt_task_work_dirs(task_uuid, sort_order, id);

CREATE TABLE IF NOT EXISTS gt_task_progress (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    task_pipeline_snapshot_uuid TEXT NOT NULL,
    task_step_uuid TEXT NOT NULL,
    local_step_uuid TEXT NOT NULL DEFAULT '',
    cloud_step_id TEXT NOT NULL DEFAULT '',
    session_uuid TEXT NOT NULL DEFAULT '',
    record_type TEXT NOT NULL CHECK(record_type IN ('initial_run', 'user_question')),
    user_prompt TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'created' CHECK(status IN ('created', 'running', 'success', 'failed', 'stopped', 'interrupted')),
    final_result TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gt_task_progress_task ON gt_task_progress(task_uuid, created_at);
CREATE INDEX IF NOT EXISTS idx_gt_task_progress_step ON gt_task_progress(task_step_uuid, created_at);

CREATE TABLE IF NOT EXISTS gt_cli_sessions (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    step_uuid TEXT NOT NULL,
    task_step_uuid TEXT NOT NULL DEFAULT '',
    task_progress_uuid TEXT NOT NULL DEFAULT '',
    conversation_uuid TEXT NOT NULL,
    parent_session_uuid TEXT NOT NULL DEFAULT '',
    run_no INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'created',
    cli_type TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    external_session_id TEXT NOT NULL DEFAULT '',
    work_dir TEXT NOT NULL DEFAULT '',
    prompt_snapshot TEXT NOT NULL DEFAULT '',
    result_status TEXT NOT NULL DEFAULT '',
    final_result TEXT NOT NULL DEFAULT '',
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
    latest_event_type TEXT NOT NULL DEFAULT '',
    latest_event_content TEXT NOT NULL DEFAULT '',
    latest_event_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(step_uuid, run_no)
);
CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_task ON gt_cli_sessions(task_uuid);
CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_conversation ON gt_cli_sessions(conversation_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_cli_sessions_progress ON gt_cli_sessions(task_progress_uuid) WHERE task_progress_uuid <> '';

CREATE TABLE IF NOT EXISTS gt_task_notifications (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL,
    task_step_uuid TEXT NOT NULL,
    step_order INTEGER NOT NULL DEFAULT 0,
    progress_uuid TEXT NOT NULL,
    session_uuid TEXT NOT NULL UNIQUE,
    terminal_status TEXT NOT NULL CHECK(terminal_status IN ('success', 'failed', 'stopped', 'interrupted')),
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    is_read INTEGER NOT NULL DEFAULT 0 CHECK(is_read IN (0, 1)),
    is_archived INTEGER NOT NULL DEFAULT 0 CHECK(is_archived IN (0, 1)),
    created_at INTEGER NOT NULL,
    read_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gt_task_notifications_inbox ON gt_task_notifications(is_archived, is_read, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gt_task_notifications_task ON gt_task_notifications(task_uuid, step_order, created_at);

CREATE TABLE IF NOT EXISTS gt_task_lanes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lane_key TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '#8c8c8c',
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_hidden INTEGER NOT NULL DEFAULT 0 CHECK(is_hidden IN (0, 1)),
    created_at INTEGER NOT NULL DEFAULT 0
);
INSERT INTO gt_task_lanes (lane_key, title, color, sort_order, is_hidden, created_at) VALUES
    ('pending', '待开始', '#8c8c8c', 0, 0, strftime('%s','now') * 1000),
    ('active', '进行中', '#3157e2', 1, 0, strftime('%s','now') * 1000),
    ('done', '已完成', '#52c41a', 2, 0, strftime('%s','now') * 1000),
    ('blocked', '已阻塞', '#f5222d', 3, 0, strftime('%s','now') * 1000);

-- Compatibility snapshot/event tables. Product history is read from gt_task_progress.

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

CREATE TABLE IF NOT EXISTS gt_device_registrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id TEXT NOT NULL,
    device_uuid TEXT NOT NULL,
    credential_ref TEXT NOT NULL DEFAULT '',
    registered_at INTEGER NOT NULL,
    UNIQUE(admin_id, device_uuid)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gt_device_registrations;
DROP TABLE IF EXISTS gt_conversation_summaries;
DROP TABLE IF EXISTS gt_session_events;
DROP TABLE IF EXISTS gt_task_lanes;
DROP TABLE IF EXISTS gt_task_notifications;
DROP TABLE IF EXISTS gt_cli_sessions;
DROP TABLE IF EXISTS gt_task_progress;
DROP TABLE IF EXISTS gt_task_project_links;
DROP TABLE IF EXISTS gt_task_work_dirs;
DROP TABLE IF EXISTS gt_task_steps;
DROP TABLE IF EXISTS gt_task_pipeline_snapshots;
DROP TABLE IF EXISTS gt_tasks;
DROP TABLE IF EXISTS gt_projects;
DROP TABLE IF EXISTS gt_pipeline_steps;
DROP TABLE IF EXISTS gt_pipelines;
-- +goose StatementEnd

