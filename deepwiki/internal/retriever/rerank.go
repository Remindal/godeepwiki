package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"deepwiki/internal/llm"
	"deepwiki/internal/model"
)

// Reranker 重排打分抽象：输入候选，输出重排后的候选（分数语义由实现方决定）。
type Reranker interface {
	Rerank(ctx context.Context, query string, candidates []model.ChunkHit) ([]model.ChunkHit, error)
}

// RerankRetriever 装饰器：内嵌基础 Retriever，Search 中对候选结果重排后截断到 topK。
// reranker 为 nil 时直通（仅按 topK 截断），是否启用由配置内部开关决定，不进入 API 契约。
type RerankRetriever struct {
	inner    Retriever
	reranker Reranker
	logger   *zap.Logger
}

func NewRerankRetriever(inner Retriever, logger *zap.Logger) *RerankRetriever {
	return &RerankRetriever{inner: inner, logger: logger}
}

// WithReranker 装配重排器（装配层启用开关；不调用则直通）。
func (r *RerankRetriever) WithReranker(rr Reranker) *RerankRetriever {
	r.reranker = rr
	return r
}

func (r *RerankRetriever) Mode() string { return r.inner.Mode() }

// Search 取 topK*4 候选 → 重排 → 截断 topK；重排失败降级为 inner 原序结果并记 WARN，不整体失败。
func (r *RerankRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	return r.SearchWithFilter(ctx, repoID, query, topK, "")
}

// SearchWithFilter 带路径过滤的重排检索（FilterSearcher 契约；inner 不支持过滤时直通）。
func (r *RerankRetriever) SearchWithFilter(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	if topK <= 0 {
		topK = 10
	}
	var candidates []model.ChunkHit
	var err error
	if fs, ok := r.inner.(FilterSearcher); ok {
		candidates, err = fs.SearchWithFilter(ctx, repoID, query, topK*4, pathFilter)
	} else {
		candidates, err = r.inner.Search(ctx, repoID, query, topK*4)
	}
	if err != nil {
		return nil, err
	}
	if r.reranker == nil || len(candidates) <= topK {
		return truncateHits(candidates, topK), nil
	}
	reranked, err := r.reranker.Rerank(ctx, query, candidates)
	if err != nil {
		r.logger.Warn("rerank failed, fallback to inner order", zap.String("repo_id", repoID), zap.Error(err))
		return truncateHits(candidates, topK), nil
	}
	return truncateHits(reranked, topK), nil
}

// llmRerankMaxCandidates 单次 LLM 重排的候选上限（超出部分按原序追加在末尾）。
const llmRerankMaxCandidates = 20

// llmRerankContentRunes 送入 LLM 的单候选正文截断长度（控制 prompt 体积）。
const llmRerankContentRunes = 500

// LLMReranker 单次 LLM 调用打分重排：候选编号 + 截断正文 → 要求返回 JSON 分数数组。
type LLMReranker struct {
	l llm.LLM
}

func NewLLMReranker(l llm.LLM) *LLMReranker { return &LLMReranker{l: l} }

// Rerank LLM 按相关性 0~10 打分后降序重排；分数缺失/解析失败返回错误（调用方降级原序）。
func (rr *LLMReranker) Rerank(ctx context.Context, query string, candidates []model.ChunkHit) ([]model.ChunkHit, error) {
	n := len(candidates)
	if n > llmRerankMaxCandidates {
		n = llmRerankMaxCandidates
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nCandidates:\n", query)
	for i := 0; i < n; i++ {
		content := []rune(candidates[i].Chunk.Content)
		if len(content) > llmRerankContentRunes {
			content = content[:llmRerankContentRunes]
		}
		fmt.Fprintf(&sb, "[%d] %s:%d-%d\n%s\n\n", i, candidates[i].Chunk.Path, candidates[i].Chunk.StartLine, candidates[i].Chunk.EndLine, string(content))
	}
	sb.WriteString("Rate each candidate's relevance to the query from 0 to 10. Respond with a JSON array of numbers only, in candidate order.")

	resp, err := rr.l.Generate(ctx, model.ChatRequest{
		Model: rr.l.ModelName(),
		Messages: []model.ChatMessage{
			{Role: "user", Content: sb.String()},
		},
		Temperature: 0,
		MaxTokens:   256,
	})
	if err != nil {
		return nil, fmt.Errorf("llm rerank generate: %w", err)
	}
	scores, err := parseScoreArray(resp.Content, n)
	if err != nil {
		return nil, err
	}

	type scored struct {
		hit   model.ChunkHit
		score float64
		idx   int
	}
	ranked := make([]scored, n)
	for i := 0; i < n; i++ {
		ranked[i] = scored{hit: candidates[i], score: scores[i], idx: i}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if ranked[j].score > ranked[i].score || (ranked[j].score == ranked[i].score && ranked[j].idx < ranked[i].idx) {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	out := make([]model.ChunkHit, 0, len(candidates))
	for _, s := range ranked {
		s.hit.Score = s.score
		out = append(out, s.hit)
	}
	return append(out, candidates[n:]...), nil
}

// parseScoreArray 从 LLM 输出中提取 JSON 数组（容忍 ```json 代码围栏与首尾杂讯）。
func parseScoreArray(content string, n int) ([]float64, error) {
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("llm rerank: no JSON array in output")
	}
	var scores []float64
	if err := json.Unmarshal([]byte(content[start:end+1]), &scores); err != nil {
		return nil, fmt.Errorf("llm rerank: parse scores: %w", err)
	}
	if len(scores) != n {
		return nil, fmt.Errorf("llm rerank: got %d scores for %d candidates", len(scores), n)
	}
	return scores, nil
}

var _ Retriever = (*RerankRetriever)(nil)
var _ Reranker = (*LLMReranker)(nil)
