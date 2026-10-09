-- +goose Up
-- +goose StatementBegin
-- 云端流水线本体只留在内存里，但本机选择的 CLI 和模型必须落库。
-- 否则重启后批量配置丢失，已经绑定该流水线的待开始任务也补不回执行方式。
CREATE TABLE IF NOT EXISTS gt_cloud_pipeline_step_execution (
    pipeline_uuid TEXT NOT NULL,
    step_uuid TEXT NOT NULL,
    cli_type TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (pipeline_uuid, step_uuid)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gt_cloud_pipeline_step_execution;
-- +goose StatementEnd
