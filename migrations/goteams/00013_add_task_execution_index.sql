-- +goose Up
-- +goose StatementBegin
-- 同一次本地任务切换执行方式后重新执行时，用递增的 execution_index 区分云端运行记录。
-- 旧记录保持 0；新的一次执行在切换事务里 +1，云端按该序号另存一行，避免覆盖历史。
ALTER TABLE gt_tasks ADD COLUMN execution_index INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE gt_tasks DROP COLUMN execution_index;
-- +goose StatementEnd
