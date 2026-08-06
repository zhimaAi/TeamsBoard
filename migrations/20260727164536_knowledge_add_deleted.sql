-- +goose Up
-- +goose StatementBegin

-- 为知识库文档表添加软删除标记列
ALTER TABLE gt_knowledge_documents ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite 不支持 DROP COLUMN（旧版本），通过重建表实现
CREATE TABLE IF NOT EXISTS gt_knowledge_documents_tmp (
    uuid TEXT PRIMARY KEY,
    folder_id INTEGER NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    file_path TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    content_hash TEXT NOT NULL DEFAULT '',
    word_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
INSERT INTO gt_knowledge_documents_tmp
    SELECT uuid, folder_id, title, file_path, tags_json, content_hash, word_count, created_at, updated_at
    FROM gt_knowledge_documents;
DROP TABLE gt_knowledge_documents;
ALTER TABLE gt_knowledge_documents_tmp RENAME TO gt_knowledge_documents;
CREATE INDEX IF NOT EXISTS idx_gt_knowledge_docs_folder ON gt_knowledge_documents(folder_id);
-- +goose StatementEnd
