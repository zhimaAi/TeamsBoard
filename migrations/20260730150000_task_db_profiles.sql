-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS gt_task_db_profiles (
    task_uuid           TEXT    NOT NULL,
    database_profile_id INTEGER NOT NULL,
    sort_order          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (task_uuid, database_profile_id)
);

CREATE INDEX IF NOT EXISTS idx_gt_task_db_profiles_task
    ON gt_task_db_profiles(task_uuid, sort_order, database_profile_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_gt_task_db_profiles_task;
DROP TABLE IF EXISTS gt_task_db_profiles;

-- +goose StatementEnd
