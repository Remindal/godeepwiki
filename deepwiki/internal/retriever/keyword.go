package retriever

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
)

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
	// TODO: 实现 OpenSearch BM25 检索，要求：
	// ① repoID 必须先过 ULID 正则（硬约束 #11），索引名经 internal/search 导出的构造函数生成
	//    （deepwiki-chunks-<repo_id 全小写>），禁止字符串拼接用户输入进查询体；
	// ② 查询体：multi_match 于 content^2, path；filter: term repo_id（索引内天然隔离可省略，保留 filter 防御）；
	//    BM25 默认排序（mapping 已声明 index.similarity.default.type=BM25）；
	//    进阶 path_filter 用 prefix 查询匹配 path.raw；
	// ③ 取 topK 命中，用 _id（chunk_id）经 chunkStore.GetByIDs 回填 Chunk；
	// ④ 尊重 ctx：OpenSearch 请求必须带 ctx，每步检查 ctx.Err()（硬约束 #4）；
	// ⑤ OpenSearch 不可用映射 50303 search_unavailable；score 为 BM25 分。
	panic("TODO: KeywordRetriever.Search not implemented")
}

var _ Retriever = (*KeywordRetriever)(nil)
