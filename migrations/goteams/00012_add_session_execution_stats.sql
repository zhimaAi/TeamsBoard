-- +goose Up
-- +goose StatementBegin
-- 会话执行统计（需求 1896 云端推送）：编排器在 CLI 事件流中尽力而为地采集，
-- 会话结束时随 finishSession 落库，并由会话同步服务随步骤会话推送到云端。
-- 存量会话无统计（保持默认 0），云端展示时按 0 隐藏。
ALTER TABLE gt_cli_sessions ADD COLUMN tool_call_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_cli_sessions ADD COLUMN files_changed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_cli_sessions ADD COLUMN lines_added INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_cli_sessions ADD COLUMN lines_deleted INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE gt_cli_sessions DROP COLUMN lines_deleted;
ALTER TABLE gt_cli_sessions DROP COLUMN lines_added;
ALTER TABLE gt_cli_sessions DROP COLUMN files_changed;
ALTER TABLE gt_cli_sessions DROP COLUMN tool_call_count;
-- +goose StatementEnd
