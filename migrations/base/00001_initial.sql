-- +goose Up
-- +goose StatementBegin

-- The base baseline is intentionally non-destructive. Historical tables or
-- files from older versions are left untouched and are not migrated or used by
-- the application-scoped task runtime.

CREATE TABLE IF NOT EXISTS gt_git_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    ssh_profile_id INTEGER NOT NULL DEFAULT 0,
    remote_work_dir TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name, ssh_profile_id, remote_work_dir)
);

CREATE TABLE IF NOT EXISTS gt_ssh_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    host TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 22,
    username TEXT NOT NULL,
    secret_ref TEXT NOT NULL DEFAULT '',
    key_path TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_database_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    db_type TEXT NOT NULL DEFAULT 'mysql',
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 3306,
    database_name TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    ssh_profile_id INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_docker_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    ssh_profile_id INTEGER NOT NULL DEFAULT 0,
    compose_file_path TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_model_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    provider_type TEXT NOT NULL DEFAULT '',
    api_base_url TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_model_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    provider_id INTEGER NOT NULL,
    model_name TEXT NOT NULL DEFAULT '',
    temperature TEXT NOT NULL DEFAULT '0.7',
    max_tokens INTEGER NOT NULL DEFAULT 4096,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_cli_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    cli_type TEXT NOT NULL DEFAULT 'codex',
    executable_path TEXT NOT NULL DEFAULT '',
    model_profile_id INTEGER NOT NULL DEFAULT 0,
    extra_args TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_client_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_key TEXT NOT NULL UNIQUE,
    config_value TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_api_collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parent_id INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

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
CREATE INDEX IF NOT EXISTS idx_gt_api_folders_collection ON gt_api_folders(collection_id, parent_id, sort_order, id);

CREATE TABLE IF NOT EXISTS gt_api_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection_id INTEGER NOT NULL DEFAULT 0,
    folder_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    method TEXT NOT NULL DEFAULT 'GET',
    url TEXT NOT NULL DEFAULT '',
    headers_json TEXT NOT NULL DEFAULT '{}',
    query_json TEXT NOT NULL DEFAULT '[]',
    auth_json TEXT NOT NULL DEFAULT '{}',
    body TEXT NOT NULL DEFAULT '',
    body_type TEXT NOT NULL DEFAULT 'none',
    body_form_json TEXT NOT NULL DEFAULT '[]',
    environment_id INTEGER NOT NULL DEFAULT 0,
    description TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gt_api_requests_collection ON gt_api_requests(collection_id);
CREATE INDEX IF NOT EXISTS idx_gt_api_requests_folder ON gt_api_requests(collection_id, folder_id, sort_order, id);

CREATE TABLE IF NOT EXISTS gt_api_environments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    variables_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(collection_id, name)
);
CREATE INDEX IF NOT EXISTS idx_gt_api_environments_collection ON gt_api_environments(collection_id, id);

CREATE TABLE IF NOT EXISTS gt_api_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_uuid TEXT NOT NULL UNIQUE,
    api_id INTEGER NOT NULL DEFAULT 0,
    collection_id INTEGER NOT NULL DEFAULT 0,
    env_id INTEGER NOT NULL DEFAULT 0,
    method TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    request_headers_json TEXT NOT NULL DEFAULT '{}',
    request_body_preview TEXT NOT NULL DEFAULT '',
    response_status INTEGER NOT NULL DEFAULT 0,
    response_headers_json TEXT NOT NULL DEFAULT '{}',
    response_body TEXT NOT NULL DEFAULT '',
    response_body_truncated INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL DEFAULT 0,
    finished_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gt_api_runs_api_time ON gt_api_runs(api_id, started_at);

CREATE TABLE IF NOT EXISTS gt_knowledge_folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    parent_id INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_knowledge_documents (
    uuid TEXT PRIMARY KEY,
    folder_id INTEGER NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    file_path TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    content_hash TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0 CHECK(deleted IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gt_knowledge_docs_folder ON gt_knowledge_documents(folder_id);

CREATE TABLE IF NOT EXISTS gt_knowledge_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_uuid TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gt_knowledge_history_doc ON gt_knowledge_history(doc_uuid, created_at);

CREATE TABLE IF NOT EXISTS gt_command_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command_type TEXT NOT NULL DEFAULT '',
    project_id INTEGER NOT NULL DEFAULT 0,
    command TEXT NOT NULL DEFAULT '',
    args_json TEXT NOT NULL DEFAULT '[]',
    work_dir TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'running',
    exit_code INTEGER NOT NULL DEFAULT -1,
    output TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gt_command_runs_type ON gt_command_runs(command_type, started_at);
CREATE INDEX IF NOT EXISTS idx_gt_command_runs_project ON gt_command_runs(project_id);

CREATE TABLE IF NOT EXISTS gt_command_output_chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_uuid TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    stream_type TEXT NOT NULL DEFAULT 'stdout',
    content TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    UNIQUE(run_uuid, sequence)
);

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

CREATE TABLE IF NOT EXISTS gt_tool_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tool_key TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    installed_version TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

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
DROP TABLE IF EXISTS gt_command_output_chunks;
DROP TABLE IF EXISTS gt_command_runs;
DROP TABLE IF EXISTS gt_knowledge_history;
DROP TABLE IF EXISTS gt_knowledge_documents;
DROP TABLE IF EXISTS gt_knowledge_folders;
DROP TABLE IF EXISTS gt_api_runs;
DROP TABLE IF EXISTS gt_api_environments;
DROP TABLE IF EXISTS gt_api_requests;
DROP TABLE IF EXISTS gt_api_folders;
DROP TABLE IF EXISTS gt_api_collections;
DROP TABLE IF EXISTS gt_client_config;
DROP TABLE IF EXISTS gt_cli_profiles;
DROP TABLE IF EXISTS gt_model_profiles;
DROP TABLE IF EXISTS gt_model_providers;
DROP TABLE IF EXISTS gt_docker_projects;
DROP TABLE IF EXISTS gt_database_profiles;
DROP TABLE IF EXISTS gt_ssh_profiles;
DROP TABLE IF EXISTS gt_git_projects;
-- +goose StatementEnd

