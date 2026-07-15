package retriever

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// RerankRetriever 装饰器：内嵌基础 Retriever，Search 中对候选结果重排后截断到 topK。
type RerankRetriever struct {
	inner  Retriever
	logger *zap.Logger
}

func NewRerankRetriever(inner Retriever, logger *zap.Logger) *RerankRetriever {
	return &RerankRetriever{inner: inner, logger: logger}
}

func (r *RerankRetriever) Mode() string { return r.inner.Mode() }

func (r *RerankRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现重排装饰，要求：
	// ① inner.Search(ctx, repoID, query, topK*4) 取候选；② 交叉编码或 LLM 重排打分；③ 截断到 topK；
	// ④ 重排失败必须降级为 inner 原序结果并记 WARN，不得整体失败。
	panic("TODO: RerankRetriever.Search not implemented")
}

var _ Retriever = (*RerankRetriever)(nil)
