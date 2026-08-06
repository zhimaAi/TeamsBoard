-- +goose Up
-- +goose StatementBegin

-- Git 项目配置表
CREATE TABLE IF NOT EXISTS gt_git_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT 'main',
    secret_ref TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name, path)
);

-- SSH 配置表
CREATE TABLE IF NOT EXISTS gt_ssh_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 22,
    username TEXT NOT NULL,
    secret_ref TEXT NOT NULL DEFAULT '',
    key_path TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- 数据库配置表
CREATE TABLE IF NOT EXISTS gt_database_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    db_type TEXT NOT NULL DEFAULT 'mysql',
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 3306,
    database_name TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    ssh_profile_id INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- Docker 项目配置表
CREATE TABLE IF NOT EXISTS gt_docker_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    compose_file_path TEXT NOT NULL,
    project_dir TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- 接口管理-集合表
CREATE TABLE IF NOT EXISTS gt_api_collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parent_id INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- 接口管理-请求表
CREATE TABLE IF NOT EXISTS gt_api_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    method TEXT NOT NULL DEFAULT 'GET',
    url TEXT NOT NULL DEFAULT '',
    headers_json TEXT NOT NULL DEFAULT '{}',
    body TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_api_requests_collection ON gt_api_requests(collection_id);

-- 接口管理-环境表
CREATE TABLE IF NOT EXISTS gt_api_environments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    variables_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- 模型服务商表
CREATE TABLE IF NOT EXISTS gt_model_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL DEFAULT '',
    api_base_url TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- 模型 Profile 表
CREATE TABLE IF NOT EXISTS gt_model_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    provider_id INTEGER NOT NULL,
    model_name TEXT NOT NULL DEFAULT '',
    temperature TEXT NOT NULL DEFAULT '0.7',
    max_tokens INTEGER NOT NULL DEFAULT 4096,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- CLI Profile 表
CREATE TABLE IF NOT EXISTS gt_cli_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    cli_type TEXT NOT NULL DEFAULT 'codex',
    executable_path TEXT NOT NULL DEFAULT '',
    model_profile_id INTEGER NOT NULL DEFAULT 0,
    extra_args TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

-- 客户端通用配置表（键值对，不含密钥）
CREATE TABLE IF NOT EXISTS gt_client_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_key TEXT NOT NULL,
    config_value TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL,
    UNIQUE(config_key)
);

-- Skill 开关表
CREATE TABLE IF NOT EXISTS gt_skill_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    skill_name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0,
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(skill_name)
);

-- 知识库文档索引表
CREATE TABLE IF NOT EXISTS gt_knowledge_documents (
    uuid TEXT PRIMARY KEY,
    folder_id INTEGER NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    file_path TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    content_hash TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_knowledge_docs_folder ON gt_knowledge_documents(folder_id);

-- 知识库文件夹表
CREATE TABLE IF NOT EXISTS gt_knowledge_folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    parent_id INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- 知识库历史版本表
CREATE TABLE IF NOT EXISTS gt_knowledge_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_uuid TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gt_knowledge_history_doc ON gt_knowledge_history(doc_uuid, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gt_knowledge_history;
DROP TABLE IF EXISTS gt_knowledge_folders;
DROP TABLE IF EXISTS gt_knowledge_documents;
DROP TABLE IF EXISTS gt_skill_settings;
DROP TABLE IF EXISTS gt_client_config;
DROP TABLE IF EXISTS gt_cli_profiles;
DROP TABLE IF EXISTS gt_model_profiles;
DROP TABLE IF EXISTS gt_model_providers;
DROP TABLE IF EXISTS gt_api_environments;
DROP TABLE IF EXISTS gt_api_requests;
DROP TABLE IF EXISTS gt_api_collections;
DROP TABLE IF EXISTS gt_docker_projects;
DROP TABLE IF EXISTS gt_database_profiles;
DROP TABLE IF EXISTS gt_ssh_profiles;
DROP TABLE IF EXISTS gt_git_projects;
-- +goose StatementEnd
