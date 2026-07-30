package retriever

import (
	"context"
	"fmt"
	"regexp"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
)

// repoIDRegex 校验 repo_id：前缀 + 26 位 Crockford Base32 ULID（硬约束 #11）。
var repoIDRegex = regexp.MustCompile(`^repo_[0-9A-HJKMNP-TV-Z]{26}$`)

// KeywordRetriever OpenSearch BM25 实现；每仓一索引 deepwiki-chunks-<repo_id 全小写>（OpenSearch
// 索引名必须小写，repo_id 含大写 ULID，统一 strings.ToLower），文档 _id = chunk_id。
type KeywordRetriever struct {
	client     *search.Client   // OpenSearch 客户端封装（internal/search，见 §5.11.5）
	chunkStore store.ChunkStore // 命中后按 chunk_id 回填 Chunk（references 校验依赖，硬约束 #15）
	logger     *zap.Logger
}

func NewKeywordRetriever(client *search.Client, chunkStore store.ChunkStore, logger *zap.Logger) *KeywordRetriever {
	return &KeywordRetriever{client: client, chunkStore: chunkStore, logger: logger}
}

func (r *KeywordRetriever) Mode() string { return "keyword" }

func (r *KeywordRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	return r.SearchWithPath(ctx, repoID, query, topK, "")
}

// SearchWithPath 带路径前缀过滤的 BM25 检索（进阶 path_filter：prefix 查询 path.raw）。
// OpenSearch 仅返回 _id/_score，Chunk 正文经 chunkStore.GetByIDs 回填；命中顺序与 BM25 排序一致。
func (r *KeywordRetriever) SearchWithPath(ctx context.Context, repoID string, query string, topK int, pathFilter string) ([]model.ChunkHit, error) {
	if !repoIDRegex.MatchString(repoID) {
		return nil, fmt.Errorf("invalid repo_id format: %q", repoID)
	}
	if topK <= 0 {
		topK = 10
	}
	hits, err := r.client.Search(ctx, repoID, query, topK, pathFilter)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrSearchUnavailable, err)
	}
	if len(hits) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ChunkID
	}
	children, err := r.chunkStore.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("backfill chunks: %w", err)
	}
	childByID := make(map[string]*model.Chunk, len(children))
	for _, c := range children {
		childByID[c.ChunkID] = c
	}

	// 父子块双层索引：命中子块 → 回查父块返回完整上下文；按父块去重保留最优子块分。
	var parentIDs []string
	seenParent := map[string]bool{}
	for _, h := range hits {
		c, ok := childByID[h.ChunkID]
		if !ok || c.ParentChunkID == "" || seenParent[c.ParentChunkID] {
			continue
		}
		seenParent[c.ParentChunkID] = true
		parentIDs = append(parentIDs, c.ParentChunkID)
	}
	parents, err := r.chunkStore.GetByIDs(ctx, parentIDs)
	if err != nil {
		return nil, fmt.Errorf("backfill parent chunks: %w", err)
	}
	parentByID := make(map[string]*model.Chunk, len(parents))
	for _, p := range parents {
		parentByID[p.ChunkID] = p
	}

	out := make([]model.ChunkHit, 0, len(hits))
	emitted := map[string]bool{}
	for _, h := range hits {
		c, ok := childByID[h.ChunkID]
		if !ok {
			r.logger.Warn("opensearch hit missing in postgres, skip", zap.String("chunk_id", h.ChunkID), zap.String("repo_id", repoID))
			continue
		}
		if c.ParentChunkID == "" {
			continue // 父块不进 BM25，理论不出现
		}
		if emitted[c.ParentChunkID] {
			continue
		}
		p, ok := parentByID[c.ParentChunkID]
		if !ok {
			r.logger.Warn("parent chunk missing in postgres, skip", zap.String("parent_chunk_id", c.ParentChunkID))
			continue
		}
		emitted[c.ParentChunkID] = true
		out = append(out, model.ChunkHit{Chunk: *p, Score: h.Score})
	}
	return out, nil
}

var _ Retriever = (*KeywordRetriever)(nil)
