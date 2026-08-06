-- +goose Up
-- +goose StatementBegin

-- 待删任务缓冲表：本地删除任务但当时未连接云端时，
-- 先记录 local_task_uuid，待 WSClient 周期刷新 / 重连时补发 task.delete 给云端。
CREATE TABLE IF NOT EXISTS gt_pending_task_deletes (
    local_task_uuid TEXT PRIMARY KEY,
    created_at      INTEGER NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS gt_pending_task_deletes;

-- +goose StatementEnd
