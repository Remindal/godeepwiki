// Package search OpenSearch 客户端与索引生命周期（总纲 R3/§4.2）：
// 每仓一索引物理隔离（删仓 = 删索引）；BM25 默认排序；opensearch-go/v4 官方客户端。
package search

import (
	"context"
	"strings"

	opensearch "github.com/opensearch-project/opensearch-go/v4"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// chunksIndexMapping chunk 索引 mapping（总纲 §4.2 权威 mapping，逐字一致，禁止改动）：
// code_analyzer 按非字母数字（保留 _ 和 .）切分并小写化；BM25 similarity；
// dev 单节点 0 副本，生产 3 节点 number_of_replicas=1（compose/部署参数化）。
const chunksIndexMapping = `{
  "settings": { "number_of_shards": 1, "number_of_replicas": 0, "analysis": {
      "analyzer": { "code_analyzer": { "type": "pattern", "pattern": "[^\\p{L}\\p{N}_.]+", "lowercase": true } } },
    "index.similarity.default.type": "BM25" },
  "mappings": { "properties": {
    "chunk_id":   { "type": "keyword" },
    "repo_id":    { "type": "keyword" },
    "path":       { "type": "text", "analyzer": "code_analyzer", "fields": { "raw": { "type": "keyword" } } },
    "content":    { "type": "text", "analyzer": "code_analyzer" },
    "language":   { "type": "keyword" },
    "start_line": { "type": "integer" },
    "end_line":   { "type": "integer" } } }
}`

// IndexName 每仓索引名：<index_prefix>-chunks-<repo_id 全小写>
//（OpenSearch 索引名必须小写；repo_id 含大写 ULID，统一 strings.ToLower，总纲 §4.2）。
func IndexName(prefix, repoID string) string {
	return prefix + "-chunks-" + strings.ToLower(repoID)
}

// Hit 关键词检索命中（KeywordRetriever 的适配输入；chunk 正文由 Postgres chunks 表回填）。
type Hit struct {
	ChunkID string
	Score   float64
}

// Client OpenSearch 客户端（索引生命周期 + 检索）。
type Client struct {
	oscli  *opensearch.Client
	prefix string // config.search.opensearch.index_prefix，默认 deepwiki
	logger *zap.Logger
}

// NewClient 建立 OpenSearch 客户端（addresses/username/password 来自配置与环境变量；
// 启动 Ping 失败即返回 error——启动失败优于带病运行，基线 §12.1）。
func NewClient(ctx context.Context, cfg config.OpenSearchConfig, logger *zap.Logger) (*Client, error) {
	// TODO: opensearch.NewClient(opensearch.Config{Addresses: cfg.Addresses, Username: cfg.Username,
	// Password: cfg.Password}) → Ping；成功日志 opensearch connected。
	panic("TODO: search.NewClient not implemented")
}

// CreateIndex 建索引（幂等：已存在跳过；mapping 用 chunksIndexMapping 常量）。
func (c *Client) CreateIndex(ctx context.Context, repoID string) error {
	// TODO: PUT /<index> body=chunksIndexMapping；400 resource_already_exists 视为成功。
	panic("TODO: Client.CreateIndex not implemented")
}

// DeleteIndex 删索引（删仓级联的外部资源步骤，总纲 §4.1：不存在视为成功；失败由调用方记 ERROR + 后台重试）。
func (c *Client) DeleteIndex(ctx context.Context, repoID string) error {
	// TODO: DELETE /<index>；404 视为成功。
	panic("TODO: Client.DeleteIndex not implemented")
}

// BulkIndex 批量写入 chunk（_id = chunk_id；Persist 阶段 Postgres 事务提交成功后再调用，顺序约定不变）。
func (c *Client) BulkIndex(ctx context.Context, repoID string, chunks []model.Chunk) error {
	// TODO: bulk API 逐条 {"index":{"_index":<index>,"_id":chunk_id}} + 文档 JSON；
	// 指标 deepwiki_opensearch_op_duration_seconds{op="bulk"} 计时。
	panic("TODO: Client.BulkIndex not implemented")
}

// Count 索引文档数（启动一致性校验：count(index) == chunks 表行数，
// 不一致 → WARN 并后台重建该仓索引，总纲 §4.2）。
func (c *Client) Count(ctx context.Context, repoID string) (int64, error) {
	// TODO: POST /<index>/_count；索引不存在返回 0（不视为错误）。
	panic("TODO: Client.Count not implemented")
}

// Search 关键词检索（KeywordRetriever 唯一实现路径，总纲 §4.2）：
// multi_match 于 content^2, path；filter: term repo_id（索引内天然隔离可省略，保留 filter 防御）；
// BM25 默认排序；pathFilter 非空时用 prefix 查询匹配 path.raw。
func (c *Client) Search(ctx context.Context, repoID, query string, topK int, pathFilter string) ([]Hit, error) {
	// TODO: 实现检索，要求：
	// ① bool.must = multi_match{query, fields:["content^2","path"]}；bool.filter = term{repo_id: repoID}；
	// ② pathFilter != "" → bool.filter 追加 prefix{"path.raw": pathFilter}；
	// ③ size=topK；解析 hits[].{_id, _score} 为 []Hit（正文由 KeywordRetriever 按 chunk_id 回 Postgres 取）；
	// ④ OpenSearch 不可用 → error 由上层映射 50303 search_unavailable（总纲 §6）；
	// ⑤ 指标 deepwiki_opensearch_op_duration_seconds{op="search"} 与 deepwiki_keyword_search_duration_seconds 计时。
	panic("TODO: Client.Search not implemented")
}

// Ping 健康探测（60s 后台探测循环用；顺带读 cluster health 与 deepwiki-* 索引数）。
func (c *Client) Ping(ctx context.Context) (clusterStatus string, indices int, err error) {
	// TODO: GET _cluster/health → status；GET _cat/indices/<prefix>-*?format=json → 计数。
	panic("TODO: Client.Ping not implemented")
}

// Close 释放资源（opensearch-go 无显式 Close，保留对称接口供优雅退出调用）。
func (c *Client) Close() error { return nil }
