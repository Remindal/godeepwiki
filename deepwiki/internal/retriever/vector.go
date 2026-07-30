package retriever

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
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

// Search query 向量化后走 pgvector；embedding 失败映射 50202 embedding_unavailable。
func (r *VectorRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	return r.SearchWithFilter(ctx, repoID, query, topK, "")
}

// SearchWithFilter 带路径前缀过滤的向量检索（进阶 path_filter，FilterSearcher 契约）。
func (r *VectorRetriever) SearchWithFilter(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	vectors, err := r.emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, &model.APIError{Code: model.CodeEmbeddingUnavailable, Message: model.MessageOf(model.CodeEmbeddingUnavailable), Err: err}
	}
	if len(vectors) == 0 {
		return nil, &model.APIError{Code: model.CodeEmbeddingUnavailable, Message: model.MessageOf(model.CodeEmbeddingUnavailable), Err: fmt.Errorf("embedder returned no vector")}
	}
	return r.SearchByVector(ctx, repoID, vectors[0], topK, pathFilter)
}

// SearchByVector pgvector HNSW 余弦检索（变更总纲 §4.1 检索 SQL 唯一实现处）。
// SET LOCAL 仅事务内有效；efSearch 为整数配置值拼接，非用户输入；pathFilter 非空时
// 按路径前缀过滤（$3 传 nil 表示不过滤）。score 为余弦相似度 [0,1]。
func (r *VectorRetriever) SearchByVector(ctx context.Context, repoID string, vector []float32, topK int, pathFilter string) ([]model.ChunkHit, error) {
	if !repoIDRegex.MatchString(repoID) {
		return nil, fmt.Errorf("invalid repo_id format: %q", repoID)
	}
	if len(vector) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}
	if topK <= 0 {
		topK = 10
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrVectorStoreUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(r.efSearch)); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrVectorStoreUnavailable, err)
	}

	var pathPattern any
	if pathFilter != "" {
		pathPattern = pathFilter + "%"
	}
	// 父子块双层检索：子块（带向量）做相似度，JOIN 父块返回完整上下文。
	rows, err := tx.Query(ctx, `
		SELECT p.chunk_id, p.path, p.start_line, p.end_line, p.language, p.content,
		       1 - (c.embedding <=> $2) AS score
		FROM chunks c
		JOIN chunks p ON c.parent_chunk_id = p.chunk_id
		WHERE c.repo_id = $1
		  AND c.parent_chunk_id IS NOT NULL
		  AND c.embedding IS NOT NULL
		  AND ($3::text IS NULL OR p.path LIKE $3)
		ORDER BY c.embedding <=> $2
		LIMIT $4
	`, repoID, pgvector.NewVector(vector), pathPattern, topK)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrVectorStoreUnavailable, err)
	}
	defer rows.Close()

	var hits []model.ChunkHit
	seen := map[string]bool{} // 按父块去重：同一父块的多个子块命中只留第一个（score 最优）
	for rows.Next() {
		var h model.ChunkHit
		if err := rows.Scan(&h.Chunk.ChunkID, &h.Chunk.Path, &h.Chunk.StartLine, &h.Chunk.EndLine, &h.Chunk.Language, &h.Chunk.Content, &h.Score); err != nil {
			return nil, fmt.Errorf("%w: %v", model.ErrVectorStoreUnavailable, err)
		}
		if seen[h.Chunk.ChunkID] {
			continue
		}
		seen[h.Chunk.ChunkID] = true
		h.Chunk.RepoID = repoID
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrVectorStoreUnavailable, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrVectorStoreUnavailable, err)
	}
	return hits, nil
}

var _ Retriever = (*VectorRetriever)(nil)
