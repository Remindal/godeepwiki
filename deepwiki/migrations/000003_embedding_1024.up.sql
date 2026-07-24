-- 000003_embedding_1024.up.sql
-- chunks.embedding 维度 1536 → 1024（切换 SiliconFlow BAAI/bge-large-zh-v1.5）。
-- 总纲 §4.1：改维度 = 新迁移 + 全量重建。dev 阶段库中无有效 chunks 数据，直接清空后改列。
-- 迁移文件一旦合入不得修改，只前进无回滚（变更总纲 §4.1）。

-- 全量重建的前置：清空旧维度数据（dev 环境无生产数据；生产环境需先备份再重建索引与向量）。
DELETE FROM chunks;

DROP INDEX IF EXISTS idx_chunks_embedding_hnsw;

ALTER TABLE chunks ALTER COLUMN embedding TYPE vector(1024);

CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw ON chunks
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 128);
