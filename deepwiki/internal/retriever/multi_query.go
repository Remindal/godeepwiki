package retriever

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"deepwiki/internal/llm"
	"deepwiki/internal/model"
)

// MultiQueryRetriever 查询重写装饰器（Multi-Query，召回率优化）：
// LLM 把原问题改写成 3 个等价查询，与原查询并行各自检索，
// 按 chunk_id 合并加权（多路命中加分），按累计分排序截断 topK。
// 改写失败自动退化为仅原查询检索（零风险降级）。
type MultiQueryRetriever struct {
	inner  Retriever
	llm    llm.LLM
	logger *zap.Logger
}

func NewMultiQueryRetriever(inner Retriever, l llm.LLM, logger *zap.Logger) *MultiQueryRetriever {
	return &MultiQueryRetriever{inner: inner, llm: l, logger: logger}
}

func (r *MultiQueryRetriever) Mode() string { return r.inner.Mode() }

// Search 默认无路径过滤（Retriever 冻结接口）。
func (r *MultiQueryRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	return r.SearchWithFilter(ctx, repoID, query, topK, "")
}

// multiHitBonus 多路命中加分系数（同一块被多个查询命中时累计加权）。
const multiHitBonus = 0.3

// SearchWithFilter 带路径前缀过滤的多查询检索（FilterSearcher 契约）。
func (r *MultiQueryRetriever) SearchWithFilter(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	if topK <= 0 {
		topK = 10
	}
	queries := r.rewriteQueries(ctx, query)

	perK := topK / 2
	if perK < 3 {
		perK = 3
	}

	type result struct {
		hits []model.ChunkHit
		err  error
	}
	ch := make(chan result, len(queries))
	for _, q := range queries {
		q := q
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					ch <- result{err: fmt.Errorf("multi-query subsearch panic: %v", rec)}
				}
			}()
			hits, err := r.searchOne(ctx, repoID, q, perK, pathFilter)
			ch <- result{hits: hits, err: err}
		}()
	}

	merged := make(map[string]*model.ChunkHit)
	var errs []error
	for range queries {
		res := <-ch
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		for _, h := range res.hits {
			if m, ok := merged[h.Chunk.ChunkID]; ok {
				m.Score += h.Score * multiHitBonus // 多路命中累计加分
			} else {
				hh := h
				merged[h.Chunk.ChunkID] = &hh
			}
		}
	}
	if len(merged) == 0 {
		if len(errs) > 0 {
			return nil, errs[0]
		}
		return nil, nil
	}

	out := make([]model.ChunkHit, 0, len(merged))
	for _, m := range merged {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Chunk.ChunkID < out[j].Chunk.ChunkID
	})
	return truncateHits(out, topK), nil
}

// searchOne 单路检索：inner 支持 FilterSearcher 则带过滤，否则走冻结接口。
func (r *MultiQueryRetriever) searchOne(ctx context.Context, repoID, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	if fs, ok := r.inner.(FilterSearcher); ok {
		return fs.SearchWithFilter(ctx, repoID, query, topK, pathFilter)
	}
	return r.inner.Search(ctx, repoID, query, topK)
}

// rewriteQueries LLM 改写原问题为 3 个等价查询（失败降级为仅原查询）。
func (r *MultiQueryRetriever) rewriteQueries(ctx context.Context, query string) []string {
	prompt := fmt.Sprintf(`将以下代码搜索问题改写成 3 个不同表述的搜索查询，保持原意、面向代码检索（可替换同义词/英文术语/补充相关模块名）。
原问题：%s
只输出 3 行，每行一个查询，不要编号、不要解释。`, query)

	resp, err := r.llm.Generate(ctx, model.ChatRequest{
		Model:       r.llm.ModelName(),
		Messages:    []model.ChatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.3,
		MaxTokens:   1024, // thinking 模型推理也占 token 预算，200 会被烧光导致改写为空
	})
	if err != nil {
		r.logger.Warn("multi-query rewrite failed, fallback to original", zap.Error(err))
		return []string{query}
	}

	variants := parseQueryLines(resp.Content, 3)
	if len(variants) == 0 {
		return []string{query}
	}
	return append([]string{query}, variants...)
}

// parseQueryLines 解析改写输出：按行切分、去序号/引号/空行/去重，最多 n 条。
func parseQueryLines(content string, n int) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-•*0123456789.、)）] ")
		line = strings.Trim(line, `"'“”`)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
		if len(out) >= n {
			break
		}
	}
	return out
}

var _ Retriever = (*MultiQueryRetriever)(nil)
var _ FilterSearcher = (*MultiQueryRetriever)(nil)
