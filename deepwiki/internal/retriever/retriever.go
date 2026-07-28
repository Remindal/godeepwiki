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

// FilterSearcher 路径前缀过滤检索（进阶 path_filter）：不进入冻结接口，
// 由各实现与装饰器满足，AskService 按需类型断言调用。
type FilterSearcher interface {
	SearchWithFilter(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error)
}
