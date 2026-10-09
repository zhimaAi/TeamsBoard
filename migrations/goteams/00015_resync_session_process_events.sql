-- +goose Up
-- +goose StatementBegin
-- 会话同步以前只上传对话正文。执行过程（思考、工具调用、工具结果）补进上报后，
-- 已经标记成功的会话不会再次推送，这里清一次同步水位，让仍留在本地的过程事件补传到云端。
UPDATE gt_cli_sessions
SET cloud_sync_status = '', cloud_synced_at = 0
WHERE cloud_sync_status = 'success';
-- +goose StatementEnd
