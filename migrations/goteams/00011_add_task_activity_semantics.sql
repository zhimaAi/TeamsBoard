-- +goose Up
-- 看板最近动态需要稳定区分用户、执行工具和系统通知；按仓库规范由应用层校验枚举值。
ALTER TABLE gt_task_progress ADD COLUMN activity_actor TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_task_progress ADD COLUMN activity_kind TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_task_created
ON gt_cli_sessions(task_uuid, created_at DESC);

-- 用户提问可以无歧义回填；历史 Vibe Coding 指派记录由应用层通过确定性 UUID 识别，
-- 避免依赖本地化展示文案。其余历史结果也由读取层兼容推导。
UPDATE gt_task_progress
SET activity_actor = 'user', activity_kind = 'user_message'
WHERE execution_mode = 'vibe_coding' AND record_type = 'user_question';
