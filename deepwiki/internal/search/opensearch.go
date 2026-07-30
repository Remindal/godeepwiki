// Package search OpenSearch 客户端与索引生命周期（总纲 R3/§4.2）：
// 每仓一索引物理隔离（删仓 = 删索引）；BM25 默认排序；opensearch-go/v4 官方客户端。
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	opensearch "github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
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

// bulkBatchSize 单次 bulk 请求的最大文档数。
const bulkBatchSize = 500

// IndexName 每仓索引名：<index_prefix>-chunks-<repo_id 全小写>
// （OpenSearch 索引名必须小写；repo_id 含大写 ULID，统一 strings.ToLower，总纲 §4.2）。
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
	api    *opensearchapi.Client
	prefix string // config.search.opensearch.index_prefix，默认 deepwiki
	logger *zap.Logger
}

// NewClient 建立 OpenSearch 客户端（addresses/username/password 来自配置与环境变量；
// 启动 Ping 失败即返回 error——启动失败优于带病运行，基线 §12.1）。
func NewClient(ctx context.Context, cfg config.OpenSearchConfig, logger *zap.Logger) (*Client, error) {
	api, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: opensearch.Config{
			Addresses: cfg.Addresses,
			Username:  cfg.Username,
			Password:  cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch new client: %w", err)
	}
	prefix := cfg.IndexPrefix
	if prefix == "" {
		prefix = "deepwiki"
	}
	c := &Client{api: api, prefix: prefix, logger: logger}
	if _, _, err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("opensearch ping: %w", err)
	}
	logger.Info("opensearch connected", zap.Strings("addresses", cfg.Addresses), zap.String("index_prefix", prefix))
	return c, nil
}

// Close 释放资源（opensearch-go 无显式 Close，保留对称接口供优雅退出调用）。
func (c *Client) Close() error { return nil }

// Ping 健康探测：cluster health status + <prefix>-* 索引数。
func (c *Client) Ping(ctx context.Context) (clusterStatus string, indices int, err error) {
	healthResp, err := c.api.Cluster.Health(ctx, &opensearchapi.ClusterHealthReq{})
	if err != nil {
		return "", 0, fmt.Errorf("opensearch cluster health: %w", err)
	}
	status := healthResp.Status
	if status == "" {
		status = "unknown"
	}
	catResp, err := c.api.Cat.Indices(ctx, &opensearchapi.CatIndicesReq{Indices: []string{c.prefix + "-*"}})
	if err != nil {
		return status, 0, fmt.Errorf("opensearch cat indices: %w", err)
	}
	return status, len(catResp.Indices), nil
}

// CreateIndex 建索引（幂等：resource_already_exists 视为成功；mapping 用 chunksIndexMapping 常量）。
func (c *Client) CreateIndex(ctx context.Context, repoID string) error {
	index := IndexName(c.prefix, repoID)
	_, err := c.api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: index,
		Body:  strings.NewReader(chunksIndexMapping),
	})
	if err != nil {
		if isErrType(err, "resource_already_exists_exception") {
			return nil
		}
		return fmt.Errorf("opensearch create index %s: %w", index, err)
	}
	return nil
}

// DeleteIndex 删索引（删仓级联的外部资源步骤；index_not_found 视为成功）。
func (c *Client) DeleteIndex(ctx context.Context, repoID string) error {
	index := IndexName(c.prefix, repoID)
	_, err := c.api.Indices.Delete(ctx, opensearchapi.IndicesDeleteReq{Indices: []string{index}})
	if err != nil {
		if isErrStatus(err, 404) {
			return nil
		}
		return fmt.Errorf("opensearch delete index %s: %w", index, err)
	}
	return nil
}

// chunkDoc 索引文档结构（与 chunksIndexMapping properties 对齐）。
type chunkDoc struct {
	ChunkID   string `json:"chunk_id"`
	RepoID    string `json:"repo_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Language  string `json:"language"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// BulkIndex 批量写入 chunk（_id = chunk_id；Persist 阶段 Postgres 事务提交成功后再调用，顺序约定不变）。
// 响应 errors=true 时聚合前几条失败原因返回错误（调用方据此记 ERROR + 后台补偿）。
func (c *Client) BulkIndex(ctx context.Context, repoID string, chunks []model.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	index := IndexName(c.prefix, repoID)
	for start := 0; start < len(chunks); start += bulkBatchSize {
		end := start + bulkBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		var buf bytes.Buffer
		for _, ch := range chunks[start:end] {
			if ch.ParentChunkID == "" {
				continue // 父子块双层索引：OpenSearch 只索引子块（父块仅供上下文，不进 BM25）
			}
			meta, err := json.Marshal(map[string]any{"index": map[string]any{"_index": index, "_id": ch.ChunkID}})
			if err != nil {
				return err
			}
			doc, err := json.Marshal(chunkDoc{
				ChunkID:   ch.ChunkID,
				RepoID:    ch.RepoID,
				Path:      ch.Path,
				Content:   ch.Content,
				Language:  ch.Language,
				StartLine: ch.StartLine,
				EndLine:   ch.EndLine,
			})
			if err != nil {
				return err
			}
			buf.Write(meta)
			buf.WriteByte('\n')
			buf.Write(doc)
			buf.WriteByte('\n')
		}
		if buf.Len() == 0 {
			continue // 本批全是父块（不入 BM25）：跳过空 bulk（OpenSearch 对空 body 报 400）
		}
		resp, err := c.api.Bulk(ctx, opensearchapi.BulkReq{Body: &buf})
		if err != nil {
			return fmt.Errorf("opensearch bulk %s: %w", index, err)
		}
		if resp.Errors {
			return fmt.Errorf("opensearch bulk %s: item failures: %s", index, bulkFailureSummary(resp.Items, 3))
		}
	}
	return nil
}

// Refresh 刷新索引使 bulk 写入立即可检索（测试与重建索引流程用）。
func (c *Client) Refresh(ctx context.Context, repoID string) error {
	_, err := c.api.Indices.Refresh(ctx, &opensearchapi.IndicesRefreshReq{Indices: []string{IndexName(c.prefix, repoID)}})
	if err != nil {
		return fmt.Errorf("opensearch refresh: %w", err)
	}
	return nil
}

// Count 索引文档数（启动一致性校验：count(index) == chunks 表行数）；索引不存在返回 0（不视为错误）。
func (c *Client) Count(ctx context.Context, repoID string) (int64, error) {
	index := IndexName(c.prefix, repoID)
	resp, err := c.api.Indices.Count(ctx, &opensearchapi.IndicesCountReq{Indices: []string{index}})
	if err != nil {
		if isErrStatus(err, 404) {
			return 0, nil
		}
		return 0, fmt.Errorf("opensearch count %s: %w", index, err)
	}
	return int64(resp.Count), nil
}

// Search 关键词检索（KeywordRetriever 唯一实现路径，总纲 §4.2）：
// multi_match 于 content^2, path；filter: term repo_id（索引内天然隔离可省略，保留 filter 防御）；
// BM25 默认排序；pathFilter 非空时用 prefix 查询匹配 path.raw。
// 仅返回 _id/_score，正文由 KeywordRetriever 按 chunk_id 回 Postgres 取（硬约束 #15）。
func (c *Client) Search(ctx context.Context, repoID, query string, topK int, pathFilter string) ([]Hit, error) {
	if topK <= 0 {
		topK = 10
	}
	filters := []any{map[string]any{"term": map[string]any{"repo_id": repoID}}}
	if pathFilter != "" {
		filters = append(filters, map[string]any{"prefix": map[string]any{"path.raw": pathFilter}})
	}
	body := map[string]any{
		"size": topK,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{map[string]any{
					"multi_match": map[string]any{
						"query":  query,
						"fields": []string{"content^2", "path"},
					},
				}},
				"filter": filters,
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	index := IndexName(c.prefix, repoID)
	resp, err := c.api.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{index},
		Body:    bytes.NewReader(raw),
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch search %s: %w", index, err)
	}
	hits := make([]Hit, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		hits = append(hits, Hit{ChunkID: h.ID, Score: float64(h.Score)})
	}
	return hits, nil
}

// isErrType 判定 OpenSearch 结构化错误类型（如 resource_already_exists_exception）。
func isErrType(err error, errType string) bool {
	var se *opensearch.StructError
	if errors.As(err, &se) {
		return se.Err.Type == errType
	}
	return false
}

// isErrStatus 判定 OpenSearch 错误 HTTP 状态码（如 404）。
func isErrStatus(err error, status int) bool {
	var se *opensearch.StructError
	if errors.As(err, &se) {
		return se.Status == status
	}
	var ste *opensearch.StringError
	if errors.As(err, &ste) {
		return ste.Status == status
	}
	return false
}

// bulkFailureSummary 聚合 bulk 响应中前 n 条失败原因（用于错误日志，禁止全量回传）。
func bulkFailureSummary(items []map[string]opensearchapi.BulkRespItem, n int) string {
	var parts []string
	for _, item := range items {
		for _, it := range item {
			if it.Error != nil {
				parts = append(parts, fmt.Sprintf("%s: %s", it.Error.Type, it.Error.Reason))
				if len(parts) >= n {
					return strings.Join(parts, "; ")
				}
			}
		}
	}
	return "unknown"
}
