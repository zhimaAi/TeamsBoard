-- +goose Up
-- +goose StatementBegin
-- 升级：gt_knowledge_documents 增加 ext 列（md/txt），并按真实相对路径建立索引（DA-02）
-- 仅 ADD COLUMN + CREATE INDEX，非破坏性；遵守 AGENTS.md：迁移文件不写 goose down、不引入 CHECK 约束。
ALTER TABLE gt_knowledge_documents ADD COLUMN ext TEXT NOT NULL DEFAULT 'md';
CREATE INDEX IF NOT EXISTS idx_gt_knowledge_docs_path ON gt_knowledge_documents(file_path, deleted);
-- +goose StatementEnd

-- 回滚参考（仅文档记录，不在迁移文件执行；AGENTS.md 规定迁移不处理 goose down）
-- ALTER TABLE gt_knowledge_documents DROP COLUMN ext;
-- DROP INDEX IF EXISTS idx_gt_knowledge_docs_path;
