-- +goose Up
-- +goose StatementBegin
ALTER TABLE gt_docker_projects RENAME TO gt_docker_projects_legacy;

CREATE TABLE gt_docker_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    ssh_profile_id INTEGER NOT NULL DEFAULT 0,
    compose_file_path TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

INSERT INTO gt_docker_projects (id, name, ssh_profile_id, compose_file_path, created_at, updated_at)
SELECT id, name, 0, compose_file_path, created_at, updated_at
FROM gt_docker_projects_legacy;

DROP TABLE gt_docker_projects_legacy;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE gt_docker_projects RENAME TO gt_docker_projects_remote;

CREATE TABLE gt_docker_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    compose_file_path TEXT NOT NULL,
    project_dir TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name)
);

INSERT INTO gt_docker_projects (id, name, compose_file_path, created_at, updated_at)
SELECT id, name, compose_file_path, created_at, updated_at
FROM gt_docker_projects_remote;

DROP TABLE gt_docker_projects_remote;
-- +goose StatementEnd
