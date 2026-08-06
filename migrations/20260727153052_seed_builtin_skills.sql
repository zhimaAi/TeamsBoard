-- +goose Up
-- +goose StatementBegin

-- 种子化内置 Skill 开关记录
-- 使用 INSERT OR IGNORE 保证幂等（skill_name 有 UNIQUE 约束）
-- 默认启用，用户可在前端关闭

INSERT OR IGNORE INTO gt_skill_settings (skill_name, enabled, config_json, created_at, updated_at) VALUES ('goteams-db', 1, '{}', 0, 0);
INSERT OR IGNORE INTO gt_skill_settings (skill_name, enabled, config_json, created_at, updated_at) VALUES ('goteams-api', 1, '{}', 0, 0);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM gt_skill_settings WHERE skill_name IN ('goteams-db','goteams-api');
-- +goose StatementEnd
