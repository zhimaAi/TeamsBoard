-- +goose Up
-- +goose StatementBegin

-- 任务与本地知识库文档的关联
CREATE TABLE IF NOT EXISTS gt_task_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_uuid TEXT NOT NULL,
    document_type TEXT NOT NULL,
    client_document_id TEXT NOT NULL DEFAULT '',
    knowledge_document_uuid TEXT NOT NULL,
    document_name TEXT NOT NULL DEFAULT '',
    document_path TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(task_uuid, document_type, client_document_id)
);

CREATE INDEX IF NOT EXISTS idx_gt_task_documents_task
    ON gt_task_documents(task_uuid, document_type, id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gt_task_documents;
-- +goose StatementEnd
