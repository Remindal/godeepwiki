-- 000004_parent_chunks.up.sql
-- 父子块双层索引（Parent Document Retrieval）：chunks 增加 parent_chunk_id。
-- 父块 = 完整上下文单元（函数/类级窗口，不嵌向量，仅供 LLM 上下文）；
-- 子块 = 小窗口（嵌向量用于检索，parent_chunk_id 指回父块）。
-- 迁移文件一旦合入不得修改，只前进无回滚（变更总纲 §4.1）。

ALTER TABLE chunks ADD COLUMN IF NOT EXISTS parent_chunk_id TEXT;

-- 子块按父块回查（向量检索 JOIN / BM25 回填用）。
CREATE INDEX IF NOT EXISTS idx_chunks_parent ON chunks(parent_chunk_id);
