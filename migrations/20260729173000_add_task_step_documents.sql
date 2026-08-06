-- +goose Up
-- +goose StatementBegin

ALTER TABLE gt_task_steps
    ADD COLUMN documents_snapshot_json TEXT NOT NULL DEFAULT '[]';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE gt_task_steps
    DROP COLUMN documents_snapshot_json;

-- +goose StatementEnd
