// Package search OpenSearch 客户端与索引生命周期封装。
// 总纲 §4.6：chunk 文档映射含 vector、knn、repo_id、chunk_id 等字段。
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// Client OpenSearch 客户端封装。
type Client struct {
	api    *opensearchapi.Client
	prefix string
	logger *zap.Logger
}

// NewClient 创建 OpenSearch 客户端并 Ping 验证。
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
	c := &Client{api: api, prefix: cfg.IndexPrefix, logger: logger}
	if _, _, err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("opensearch ping: %w", err)
	}
	logger.Info("opensearch connected", zap.Strings("addresses", cfg.Addresses), zap.String("index_prefix", cfg.IndexPrefix))
	return c, nil
}

// Close 关闭客户端（OpenSearch 客户端无显式 close，占位）。
func (c *Client) Close() error { return nil }

// Ping 返回集群健康状态与 deepwiki 索引数量。
func (c *Client) Ping(ctx context.Context) (string, int, error) {
	healthResp, err := c.api.Cluster.Health(ctx, &opensearchapi.ClusterHealthReq{})
	if err != nil {
		return "", 0, fmt.Errorf("opensearch health: %w", err)
	}
	status := healthResp.Status
	if status == "" {
		status = "unknown"
	}

	catResp, err := c.api.Cat.Indices(ctx, &opensearchapi.CatIndicesReq{Indices: []string{c.prefix + "*"}})
	if err != nil {
		return status, 0, fmt.Errorf("opensearch cat indices: %w", err)
	}
	return status, len(catResp.Indices), nil
}

// IndexName 返回带前缀的索引名。
func (c *Client) IndexName(name string) string { return c.prefix + name }

// BulkIndexer OpenSearch bulk indexer 包装（总纲 §4.6 推荐 batch bulk）。
func (c *Client) BulkIndexer() BulkIndexer { return newBulkIndexer(c) }

type BulkIndexer interface {
	Add(action BulkAction, docID string, doc any) error
	Flush(ctx context.Context) error
	Close() error
}

type BulkAction string

const (
	BulkIndex  BulkAction = "index"
	BulkCreate BulkAction = "create"
	BulkUpdate BulkAction = "update"
	BulkDelete BulkAction = "delete"
)

type bulkIndexer struct {
	c       *Client
	buf     bytes.Buffer
	pending int
}

func newBulkIndexer(c *Client) *bulkIndexer { return &bulkIndexer{c: c} }

func (bi *bulkIndexer) Add(action BulkAction, docID string, doc any) error {
	meta := map[string]any{"_index": bi.c.IndexName("chunks")}
	if docID != "" {
		meta["_id"] = docID
	}
	metaLine, err := json.Marshal(map[string]any{string(action): meta})
	if err != nil {
		return err
	}
	bi.buf.Write(metaLine)
	bi.buf.WriteByte('\n')
	if action != BulkDelete {
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		bi.buf.Write(data)
		bi.buf.WriteByte('\n')
	}
	bi.pending++
	return nil
}

func (bi *bulkIndexer) Flush(ctx context.Context) error {
	if bi.pending == 0 {
		return nil
	}
	_, err := bi.c.api.Bulk(ctx, opensearchapi.BulkReq{
		Body: bytes.NewReader(bi.buf.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("opensearch bulk: %w", err)
	}
	bi.buf.Reset()
	bi.pending = 0
	return nil
}

func (bi *bulkIndexer) Close() error { return bi.Flush(context.Background()) }

// CreateIndex 创建 chunk 索引（含 knn 映射）。
func (c *Client) CreateIndex(ctx context.Context, name string, dim int) error {
	panic("TODO: Client.CreateIndex not implemented")
}

// DeleteIndex 删除索引。
func (c *Client) DeleteIndex(ctx context.Context, name string) error {
	panic("TODO: Client.DeleteIndex not implemented")
}

// BulkIndex 批量索引 chunks。
func (c *Client) BulkIndex(ctx context.Context, index string, docs []IndexDoc) error {
	panic("TODO: Client.BulkIndex not implemented")
}

// Count 统计索引文档数。
func (c *Client) Count(ctx context.Context, index string) (int64, error) {
	panic("TODO: Client.Count not implemented")
}

// Search 语义检索（k-NN + 过滤）。
func (c *Client) Search(ctx context.Context, index string, vec []float32, k int, filters map[string]any) (*SearchResult, error) {
	panic("TODO: Client.Search not implemented")
}

// SearchResult 检索结果。
type SearchResult struct {
	Hits []Hit `json:"hits"`
}

// Hit 单条命中。
type Hit struct {
	ID     string         `json:"_id"`
	Score  float64        `json:"_score"`
	Source map[string]any `json:"_source"`
}

// IndexDoc 待索引文档。
type IndexDoc struct {
	ID     string
	Vector []float32
	Fields map[string]any
}

