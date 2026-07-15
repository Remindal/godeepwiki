-- 000001_init.up.sql —— DeepWiki 初始 schema（PostgreSQL 16 + pgvector）。
-- 只前进迁移：本文件合入后禁止修改；schema 变更只能新增更高序号的 .up.sql。
-- schema_migrations 版本表由 golang-migrate 自建自管，本文件不创建。

-- pgvector 扩展（向量列与 HNSW 索引依赖；镜像必须为 pgvector/pgvector:pg16）
CREATE EXTENSION IF NOT EXISTS vector;

-- 仓库
CREATE TABLE IF NOT EXISTS repos (
    repo_id     TEXT PRIMARY KEY,                 -- repo_ + ULID
    repo_url    TEXT NOT NULL,
    branch      TEXT NOT NULL,
    commit_hash TEXT NOT NULL DEFAULT '',
    local_path  TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'ingesting' -- ingesting|ready|error
                CHECK (state IN ('ingesting','ready','error')),
    stats_json  JSONB NOT NULL DEFAULT '{"files":0,"chunks":0,"tokens":0}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_url, branch)                     -- ingest 幂等的存储层兜底
);

-- 任务（三类任务共用；状态单一字段 state；任务状态唯一来源，硬约束 #3）
CREATE TABLE IF NOT EXISTS tasks (
    task_id              TEXT PRIMARY KEY,        -- tsk_ + ULID
    type                 TEXT NOT NULL
                         CHECK (type IN ('ingest','refresh','wiki')),
    repo_id              TEXT REFERENCES repos(repo_id) ON DELETE SET NULL,
    state                TEXT NOT NULL
                         CHECK (state IN ('pending','cloning','parsing','chunking',
                                          'embedding','persisting','outlining',
                                          'generating','fetching','diffing',
                                          'completed','failed','cancelled')),
    progress_json        JSONB NOT NULL DEFAULT '{"current":0,"total":0,"percent":0}'::jsonb,
    stats_json           JSONB NOT NULL DEFAULT '{"files":0,"chunks":0,"tokens":0}'::jsonb,
    error_json           JSONB,                   -- {code,message,stage} 或 NULL
    queue_position       INTEGER NOT NULL DEFAULT 0,
    cancel_flag          INTEGER NOT NULL DEFAULT 0,
    request_payload_json JSONB,                   -- 原始请求快照（重试/审计）
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at           TIMESTAMPTZ,
    finished_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tasks_state   ON tasks(state);
CREATE INDEX IF NOT EXISTS idx_tasks_repo    ON tasks(repo_id);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);

-- 代码/文档分块（向量内联于 chunks.embedding，pgvector 列；VectorStore 与 VectorRetriever 的物理载体）
CREATE TABLE IF NOT EXISTS chunks (
    chunk_id        TEXT PRIMARY KEY,             -- chk_ + ULID
    repo_id         TEXT NOT NULL
                    REFERENCES repos(repo_id) ON DELETE CASCADE,
    path            TEXT NOT NULL,                -- 仓库内相对路径
    start_line      INTEGER NOT NULL,
    end_line        INTEGER NOT NULL,
    language        TEXT NOT NULL,
    content         TEXT NOT NULL,
    file_hash       TEXT NOT NULL DEFAULT '',     -- 文件 sha256[:16]，refresh diff 用
    embedding_model TEXT NOT NULL DEFAULT '',     -- 维度一致性校验用（硬约束 #14）
    embedding       vector(1536),                 -- pgvector 列；维度建表定型（storage.vector.dimensions），
                                                  -- 改维度 = 新迁移 + 全量重建；维度不符写入直接拒绝（#14 第二道防线）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chunks_repo      ON chunks(repo_id);
CREATE INDEX IF NOT EXISTS idx_chunks_repo_path ON chunks(repo_id, path);
-- HNSW 近似最近邻索引（余弦距离；查询时 SET LOCAL hnsw.ef_search = 64）
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw ON chunks
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 128);

-- Wiki（TOC 与页面同表，kind 区分；一仓一行 kind='toc' + N 行 kind='page'）
CREATE TABLE IF NOT EXISTS wiki_pages (
    repo_id     TEXT NOT NULL
                REFERENCES repos(repo_id) ON DELETE CASCADE,
    slug        TEXT NOT NULL,                    -- 仓内标识，如 overview / module-api
    kind        TEXT NOT NULL CHECK (kind IN ('toc','page')),
    title       TEXT NOT NULL DEFAULT '',
    parent_slug TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    content_md  TEXT NOT NULL DEFAULT '',
    toc_json    JSONB,                            -- 仅 kind='toc' 行存放 TOC JSON
    task_id     TEXT REFERENCES tasks(task_id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (repo_id, slug)
);

-- API 密钥（只存哈希，硬约束 #2：SHA-256(salt‖key)；环境变量引导 → 本表 → Redis 60s 缓存二级查找）
CREATE TABLE IF NOT EXISTS api_keys (
    key_id      TEXT PRIMARY KEY,                 -- key_ + ULID
    name        TEXT NOT NULL DEFAULT '',
    key_hash    TEXT NOT NULL UNIQUE,             -- SHA-256(salt ‖ key) 十六进制
    salt        TEXT NOT NULL,
    is_admin    BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
