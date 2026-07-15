package retriever

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/embed"
	"deepwiki/internal/model"
)

// VectorRetriever 向量检索实现：query 向量化 → pgvector HNSW 检索。
// 持有 pgxpool 直连 chunks 表（变更总纲 §4.1：检索 SQL 唯一实现处）。
type VectorRetriever struct {
	pool     *pgxpool.Pool
	emb      embed.Embedder
	efSearch int // storage.vector.ef_search，默认 64（HNSW 查询精度/延迟权衡，可热更新）
	logger   *zap.Logger
}

func NewVectorRetriever(pool *pgxpool.Pool, emb embed.Embedder, efSearch int, logger *zap.Logger) *VectorRetriever {
	if efSearch <= 0 {
		efSearch = 64
	}
	return &VectorRetriever{pool: pool, emb: emb, efSearch: efSearch, logger: logger}
}

func (r *VectorRetriever) Mode() string { return "embedding" }

func (r *VectorRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现 pgvector 检索，要求：
	// ① repoID 必须先过 ULID 正则（硬约束 #11）；
	// ② r.emb.Embed(ctx, []string{query}) 取第一条向量，失败映射 50202 embedding_unavailable；
	// ③ 必须在事务内执行下列 SQL（SET LOCAL 仅事务内有效；efSearch 为整数配置值拼接，非用户输入），
	//    变更总纲 §4.1 检索 SQL 全文：
	//      SET LOCAL hnsw.ef_search = 64;
	//      SELECT chunk_id, path, start_line, end_line, language, content,
	//             1 - (embedding <=> $2) AS score
	//      FROM chunks
	//      WHERE repo_id = $1
	//        AND ($3::text IS NULL OR path LIKE $3)   -- 按文件路径过滤（进阶要求）
	//      ORDER BY embedding <=> $2
	//      LIMIT $4;
	//    $3 传 nil 表示不按路径过滤；
	// ④ 查询向量绑定用 pgvector.NewVector(vec)；维度不符会被 vector(1536) 列类型直接拒绝（硬约束 #14 第二道防线）；
	// ⑤ score 为余弦相似度 [0,1]；DB 查询失败映射 50203 vector_store_unavailable。
	panic("TODO: VectorRetriever.Search not implemented")
}

var _ Retriever = (*VectorRetriever)(nil)
