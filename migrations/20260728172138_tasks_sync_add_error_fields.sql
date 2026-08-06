-- +goose Up
-- +goose StatementBegin
-- 任务同步状态表补充错误回写字段。
-- ws_client.go 在 handleTaskSnapshotACK 清除 dirty 标记时会回写 last_error_code / last_error_message，
-- 但 init_tables 建表时漏建这两列，导致 "no such column: last_error_code" 错误。
-- 仅新增列、默认值空串，不影响已有数据，向前兼容。

ALTER TABLE gt_task_sync_state
    ADD COLUMN last_error_code TEXT NOT NULL DEFAULT '';

ALTER TABLE gt_task_sync_state
    ADD COLUMN last_error_message TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE gt_task_sync_state
    DROP COLUMN last_error_code;

ALTER TABLE gt_task_sync_state
    DROP COLUMN last_error_message;
-- +goose StatementEnd
