-- +goose Up
-- Vibe Coding 外部编辑器的当前会话绑定由应用层校验，按仓库规范不使用 CHECK 约束。
ALTER TABLE gt_tasks ADD COLUMN vibe_coding_session_json TEXT NOT NULL DEFAULT '{}';
