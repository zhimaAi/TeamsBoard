-- +goose Up
-- +goose StatementBegin
-- 对话正文改为与客户端一致：用户气泡用 user_prompt，最终回复和 Token 用会话记录，
-- 执行过程按本地事件原文上传。已成功的水位里还是旧投影，清一次让云端重收。
UPDATE gt_cli_sessions
SET cloud_sync_status = '', cloud_synced_at = 0
WHERE cloud_sync_status = 'success';
-- +goose StatementEnd
