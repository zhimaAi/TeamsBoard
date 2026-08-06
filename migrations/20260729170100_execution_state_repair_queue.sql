-- +goose Up
-- +goose StatementBegin
-- 登录初始化时由 Orchestrator 读取该队列，按“最新一轮”重算状态并通过
-- 标准 SyncTaskNow 链路生成新的 revision/hash/event，避免直接在迁移 SQL
-- 中伪造同步元数据，也确保云端已缓存的旧失败状态被刷新。
CREATE TABLE IF NOT EXISTS gt_execution_state_repair_queue (
    task_uuid TEXT PRIMARY KEY
);

INSERT OR IGNORE INTO gt_execution_state_repair_queue (task_uuid)
SELECT DISTINCT task_uuid
FROM gt_cli_sessions;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gt_execution_state_repair_queue;
-- +goose StatementEnd
