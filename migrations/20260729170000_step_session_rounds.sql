-- +goose Up
-- +goose StatementBegin
-- run_no 代表同一工作流步骤的执行轮次。旧版本按 conversation_uuid 编号，
-- 导致每次“重新执行步骤”创建新对话后都重新显示为第 1 轮。
WITH ranked AS (
    SELECT
        uuid,
        ROW_NUMBER() OVER (
            PARTITION BY step_uuid
            ORDER BY created_at ASC, rowid ASC
        ) AS new_run_no
    FROM gt_cli_sessions
)
UPDATE gt_cli_sessions
SET run_no = (
    SELECT ranked.new_run_no
    FROM ranked
    WHERE ranked.uuid = gt_cli_sessions.uuid
);

-- 会话摘要使用与 Session 相同的步骤轮次号，保证云端历史顺序一致。
UPDATE gt_conversation_summaries
SET round_no = (
    SELECT s.run_no
    FROM gt_cli_sessions s
    WHERE s.uuid = gt_conversation_summaries.session_uuid
)
WHERE EXISTS (
    SELECT 1
    FROM gt_cli_sessions s
    WHERE s.uuid = gt_conversation_summaries.session_uuid
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gt_cli_sessions_step_run_no
    ON gt_cli_sessions(step_uuid, run_no);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_gt_cli_sessions_step_run_no;

WITH ranked AS (
    SELECT
        uuid,
        ROW_NUMBER() OVER (
            PARTITION BY conversation_uuid
            ORDER BY created_at ASC, rowid ASC
        ) AS old_run_no
    FROM gt_cli_sessions
)
UPDATE gt_cli_sessions
SET run_no = (
    SELECT ranked.old_run_no
    FROM ranked
    WHERE ranked.uuid = gt_cli_sessions.uuid
);

UPDATE gt_conversation_summaries
SET round_no = (
    SELECT s.run_no
    FROM gt_cli_sessions s
    WHERE s.uuid = gt_conversation_summaries.session_uuid
)
WHERE EXISTS (
    SELECT 1
    FROM gt_cli_sessions s
    WHERE s.uuid = gt_conversation_summaries.session_uuid
);
-- +goose StatementEnd
