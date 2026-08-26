-- +goose Up
-- +goose StatementBegin
-- 云端任务会话同步：gt_cli_sessions 每创建/更新一条云端任务会话记录，
-- 由独立的异步同步服务按任务顺序推送到云端。
-- cloud_sync_status：最近一次同步结果（''=从未同步，success=成功，failed=失败）；
-- cloud_sync_error：失败原因（HTTP 状态码或网络错误描述）；
-- cloud_synced_at：最近一次同步成功时会话的 updated_at 水位，updated_at 超过水位即视为有新变更待同步。
ALTER TABLE gt_cli_sessions ADD COLUMN cloud_sync_status TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_cli_sessions ADD COLUMN cloud_sync_error TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_cli_sessions ADD COLUMN cloud_synced_at INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_task_created ON gt_cli_sessions(task_uuid, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_gt_cli_sessions_task_created;
ALTER TABLE gt_cli_sessions DROP COLUMN cloud_synced_at;
ALTER TABLE gt_cli_sessions DROP COLUMN cloud_sync_error;
ALTER TABLE gt_cli_sessions DROP COLUMN cloud_sync_status;
-- +goose StatementEnd