package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// VectorStore 向量存储抽象（基线 §7，冻结签名）。
type VectorStore interface {
	Upsert(ctx context.Context, chunks []model.Chunk) error
	Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error)
	DeleteByRepo(ctx context.Context, repoID string) error
}

// pgVectorStore pgvector 实现：embedding vector(1536) 列 + HNSW 索引（变更总纲 §4.1）。
type pgVectorStore struct {
	pool     *pgxpool.Pool
	efSearch int // storage.vector.ef_search，默认 64（可热更新）
	logger   *zap.Logger
}

func NewVectorStore(db *DB, efSearch int, logger *zap.Logger) VectorStore {
	if efSearch <= 0 {
		efSearch = 64
	}
	return &pgVectorStore{pool: db.Pool(), efSearch: efSearch, logger: logger}
}

var _ VectorStore = (*pgVectorStore)(nil)

func (s *pgVectorStore) Upsert(ctx context.Context, chunks []model.Chunk) error {
	// TODO: 与 chunk 行同事务的批量 UPSERT，要求：
	// ① INSERT INTO chunks (...) VALUES (...) ON CONFLICT (chunk_id) DO UPDATE SET
	//    embedding = EXCLUDED.embedding, embedding_model = EXCLUDED.embedding_model；
	// ② 向量绑定用 pgvector.NewVector(c.Vector)；Vector 为 nil 的行跳过 embedding 更新；
	// ③ 维度不符由 vector(1536) 列类型直接拒绝（硬约束 #14 第二道防线），禁止应用层静默截断/补零；
	// ④ 批次按 500 条分批，全部参数化 $n 占位（硬约束 #11）。
	_ = pgvector.NewVector // 提示：本文件统一用 pgvector-go 类型绑定向量
	panic("TODO: pgVectorStore.Upsert not implemented")
}

func (s *pgVectorStore) Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error) {
	// TODO: 在事务内执行变更总纲 §4.1 检索 SQL（SET LOCAL 仅事务内有效；efSearch 为整数配置值拼接，非用户输入），SQL 全文：
	//   SET LOCAL hnsw.ef_search = 64;
	//   SELECT chunk_id, path, start_line, end_line, language, content,
	//          1 - (embedding <=> $2) AS score
	//   FROM chunks
	//   WHERE repo_id = $1
	//     AND ($3::text IS NULL OR path LIKE $3)   -- 按文件路径过滤（进阶要求）
	//   ORDER BY embedding <=> $2
	//   LIMIT $4;
	// 要求：① repoID 先过 ULID 正则（硬约束 #11）；② $2 绑定 pgvector.NewVector(vector)；
	// ③ $3 传 nil 表示不按路径过滤；④ score 为余弦相似度 [0,1]；
	// ⑤ 查询失败映射 model.ErrVectorStoreUnavailable（→ 50203）；
	// ⑥ 注意：ask 默认路径走 internal/retriever.VectorRetriever（检索 SQL 唯一实现处），
	//    本方法供 service 层不经 retriever 的直连场景使用，两处 SQL 必须保持逐字一致。
	panic("TODO: pgVectorStore.Search not implemented")
}

func (s *pgVectorStore) DeleteByRepo(ctx context.Context, repoID string) error {
	// TODO: DELETE FROM chunks WHERE repo_id=$1（向量内联于 chunks 表，删除即生效）；
	// 仓级删除常规路径走 RepoStore.Delete 的 ON DELETE CASCADE，本方法供 refresh 全量重建用。
	panic("TODO: pgVectorStore.DeleteByRepo not implemented")
}
