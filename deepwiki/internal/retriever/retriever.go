// Package retriever 可插拔检索抽象与实现（keyword/embedding/hybrid + rerank 装饰器）。
package retriever

import (
	"context"

	"deepwiki/internal/model"
)

// Retriever 检索抽象（基线 §7，冻结签名）。
type Retriever interface {
	Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error)
	Mode() string // keyword|embedding|hybrid
}
