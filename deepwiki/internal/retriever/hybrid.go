package retriever

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	if rrfK <= 0 {
		rrfK = 60
	}
	return &HybridRetriever{keyword: keyword, vector: vector, rrfK: rrfK, logger: logger}
}

func (r *HybridRetriever) Mode() string { return "hybrid" }

// Search 两路并行检索（goroutine defer recover + 传 ctx），RRF 融合：
// score = Σ 1/(rrfK + rank)，rank 从 1 起；按 chunk_id 合并取最高 RRF 分，降序截断 topK。
// 任一路失败降级为另一路结果并记 WARN，不整体失败；两路均失败才返回错误。
func (r *HybridRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	return r.search(ctx, repoID, query, topK, "")
}

// SearchWithFilter 带路径前缀过滤的混合检索（FilterSearcher 契约，两路均带过滤）。
func (r *HybridRetriever) SearchWithFilter(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	return r.search(ctx, repoID, query, topK, pathFilter)
}

func (r *HybridRetriever) search(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	if topK <= 0 {
		topK = 10
	}
	type result struct {
		hits []model.ChunkHit
		err  error
	}
	run := func(fn func() ([]model.ChunkHit, error)) (res result) {
		defer func() {
			if rec := recover(); rec != nil {
				res.err = fmt.Errorf("retriever panic: %v", rec)
			}
		}()
		res.hits, res.err = fn()
		return res
	}
	kwCh := make(chan result, 1)
	vecCh := make(chan result, 1)
	go func() {
		kwCh <- run(func() ([]model.ChunkHit, error) { return r.keyword.SearchWithPath(ctx, repoID, query, topK, pathFilter) })
	}()
	go func() {
		vecCh <- run(func() ([]model.ChunkHit, error) { return r.vector.SearchWithFilter(ctx, repoID, query, topK, pathFilter) })
	}()
	kw, vec := <-kwCh, <-vecCh

	if kw.err != nil && vec.err != nil {
		return nil, fmt.Errorf("keyword: %v; vector: %v", kw.err, vec.err)
	}
	if kw.err != nil {
		r.logger.Warn("hybrid: keyword path failed, degrade to vector", zap.String("repo_id", repoID), zap.Error(kw.err))
		return truncateHits(vec.hits, topK), nil
	}
	if vec.err != nil {
		r.logger.Warn("hybrid: vector path failed, degrade to keyword", zap.String("repo_id", repoID), zap.Error(vec.err))
		return truncateHits(kw.hits, topK), nil
	}
	return rrfFuse(r.rrfK, topK, kw.hits, vec.hits), nil
}

// rrfFuse RRF 融合多路结果：score = Σ 1/(k + rank)（rank 从 1 起）；
// 同 chunk_id 合并取累计 RRF 分（chunk 正文取首见），降序截断 topK。
func rrfFuse(k, topK int, lists ...[]model.ChunkHit) []model.ChunkHit {
	if k <= 0 {
		k = 60
	}
	type merged struct {
		hit   model.ChunkHit
		score float64
	}
	byID := make(map[string]*merged)
	for _, hits := range lists {
		for rank, h := range hits {
			m, ok := byID[h.Chunk.ChunkID]
			if !ok {
				m = &merged{hit: h}
				byID[h.Chunk.ChunkID] = m
			}
			m.score += 1.0 / float64(k+rank+1)
		}
	}
	out := make([]model.ChunkHit, 0, len(byID))
	for _, m := range byID {
		// 语言权重：docs 类（markdown/text 等）降权，避免代码实现被文档挤出 topK
		// （BM25 对自然语言文档天然友好，代码片段需要权重补偿）。
		m.hit.Score = m.score * langWeight(m.hit.Chunk.Language)
		out = append(out, m.hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Chunk.ChunkID < out[j].Chunk.ChunkID // 分数相同时按 ID 定序，保证结果稳定
	})
	return truncateHits(out, topK)
}

// langWeight 按 chunk 语言返回排序权重：代码 1.0，文档 0.75。
func langWeight(lang string) float64 {
	switch strings.ToLower(lang) {
	case "markdown", "md", "text", "txt", "rst", "adoc", "html", "":
		return 0.75
	default:
		return 1.0
	}
}

// truncateHits 截断到 topK；topK ≤ 0 表示不截断。
func truncateHits(hits []model.ChunkHit, topK int) []model.ChunkHit {
	if topK > 0 && len(hits) > topK {
		return hits[:topK]
	}
	return hits
}

var _ Retriever = (*HybridRetriever)(nil)
