-- +goose Up
ALTER TABLE gt_tasks ADD COLUMN work_dir TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE gt_tasks DROP COLUMN work_dir;
