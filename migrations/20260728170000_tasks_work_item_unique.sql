-- +goose Up
-- +goose StatementBegin
-- 同一需求/缺陷在一个用户(admin_id, user_id)下只能属于一个任务。
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_tasks_work_item_uniq
    ON gt_tasks(admin_id, user_id, work_item_type, work_item_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_gt_tasks_work_item_uniq;
-- +goose StatementEnd
