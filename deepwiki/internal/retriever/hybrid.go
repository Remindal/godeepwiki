package retriever

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// HybridRetriever RRF 融合（基线 §2.1，rrf_k 默认 60）。
type HybridRetriever struct {
	keyword *KeywordRetriever
	vector  *VectorRetriever
	rrfK    int
	logger  *zap.Logger
}

func NewHybridRetriever(keyword *KeywordRetriever, vector *VectorRetriever, rrfK int, logger *zap.Logger) *HybridRetriever {
	return &HybridRetriever{keyword: keyword, vector: vector, rrfK: rrfK, logger: logger}
}

func (r *HybridRetriever) Mode() string { return "hybrid" }

func (r *HybridRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现 RRF 融合，要求：
	// ① 两路检索（OpenSearch BM25 + pgvector HNSW）可并行，但 goroutine 必须 defer recover() 且传 ctx（硬约束 #4）；
	// ② 融合分 score = Σ 1/(rrfK + rank)，按 chunk_id 合并，降序取 topK；
	// ③ 任一路失败降级为另一路结果并记 WARN，不整体失败；两路均失败才返回错误。
	panic("TODO: HybridRetriever.Search not implemented")
}

var _ Retriever = (*HybridRetriever)(nil)
