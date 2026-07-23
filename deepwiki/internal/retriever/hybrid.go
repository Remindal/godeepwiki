package retriever

import (
	"context"
	"fmt"
	"sort"

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
		kwCh <- run(func() ([]model.ChunkHit, error) { return r.keyword.Search(ctx, repoID, query, topK) })
	}()
	go func() {
		vecCh <- run(func() ([]model.ChunkHit, error) { return r.vector.Search(ctx, repoID, query, topK) })
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
		m.hit.Score = m.score
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

// truncateHits 截断到 topK；topK ≤ 0 表示不截断。
func truncateHits(hits []model.ChunkHit, topK int) []model.ChunkHit {
	if topK > 0 && len(hits) > topK {
		return hits[:topK]
	}
	return hits
}

var _ Retriever = (*HybridRetriever)(nil)
