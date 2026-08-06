-- +goose Up
-- +goose StatementBegin
ALTER TABLE gt_git_projects RENAME TO gt_git_projects_legacy;

CREATE TABLE gt_git_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    ssh_profile_id INTEGER NOT NULL DEFAULT 0,
    remote_work_dir TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name, ssh_profile_id, remote_work_dir)
);

INSERT INTO gt_git_projects (id, name, ssh_profile_id, remote_work_dir, created_at, updated_at)
SELECT id, name, 0, path, created_at, updated_at
FROM gt_git_projects_legacy;

DROP TABLE gt_git_projects_legacy;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE gt_git_projects RENAME TO gt_git_projects_remote;

CREATE TABLE gt_git_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT 'main',
    secret_ref TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(name, path)
);

INSERT INTO gt_git_projects (id, name, path, default_branch, secret_ref, created_at, updated_at)
SELECT id, name, remote_work_dir, 'main', '', created_at, updated_at
FROM gt_git_projects_remote;

DROP TABLE gt_git_projects_remote;
-- +goose StatementEnd
