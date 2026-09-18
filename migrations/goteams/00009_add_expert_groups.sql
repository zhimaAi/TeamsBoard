-- +goose Up
-- 专家团定义允许先保存草稿；完整性由应用层在任务指派时校验。
CREATE TABLE IF NOT EXISTS gt_expert_groups (
    uuid TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gt_expert_group_members (
    uuid TEXT PRIMARY KEY,
    expert_group_uuid TEXT NOT NULL,
    member_role TEXT NOT NULL DEFAULT 'member',
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    cli_type TEXT NOT NULL DEFAULT '',
    model_name TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gt_expert_group_members_group
    ON gt_expert_group_members(expert_group_uuid, member_role, created_at, uuid);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_expert_group_one_leader
    ON gt_expert_group_members(expert_group_uuid)
    WHERE member_role = 'leader';

CREATE TABLE IF NOT EXISTS gt_task_expert_group_snapshots (
    uuid TEXT PRIMARY KEY,
    task_uuid TEXT NOT NULL UNIQUE,
    source_expert_group_uuid TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar TEXT NOT NULL DEFAULT '',
    leader_task_step_uuid TEXT NOT NULL DEFAULT '',
    snapshot_hash TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

ALTER TABLE gt_tasks ADD COLUMN selected_expert_group_uuid TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_tasks ADD COLUMN expert_group_snapshot_uuid TEXT NOT NULL DEFAULT '';

ALTER TABLE gt_task_steps ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'pipeline';
ALTER TABLE gt_task_steps ADD COLUMN member_role TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_task_steps ADD COLUMN task_expert_group_snapshot_uuid TEXT NOT NULL DEFAULT '';

ALTER TABLE gt_task_progress ADD COLUMN task_expert_group_snapshot_uuid TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_task_progress ADD COLUMN dispatch_status TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_task_progress ADD COLUMN dispatch_message TEXT NOT NULL DEFAULT '';

ALTER TABLE gt_cli_sessions ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'pipeline';
ALTER TABLE gt_cli_sessions ADD COLUMN expert_chain_uuid TEXT NOT NULL DEFAULT '';
ALTER TABLE gt_cli_sessions ADD COLUMN expert_chain_round INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gt_cli_sessions ADD COLUMN trigger_session_uuid TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_gt_cli_sessions_expert_chain
    ON gt_cli_sessions(task_uuid, expert_chain_uuid, expert_chain_round);
