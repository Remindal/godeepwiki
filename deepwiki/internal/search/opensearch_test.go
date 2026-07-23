package search

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

func TestIndexName(t *testing.T) {
	got := IndexName("deepwiki", "repo_01J2X9K7QZ0ABCDEFGHJKMNQ")
	want := "deepwiki-chunks-repo_01j2x9k7qz0abcdefghjkmnq"
	if got != want {
		t.Fatalf("IndexName = %q, want %q", got, want)
	}
}

// testClient 连接本地 OpenSearch；不可达时 Skip（集成测试依赖 docker compose 基础设施）。
func testClient(t *testing.T) *Client {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "localhost:9200", 2*time.Second)
	if err != nil {
		t.Skipf("opensearch unreachable: %v", err)
	}
	_ = conn.Close()
	c, err := NewClient(context.Background(), config.OpenSearchConfig{
		Addresses:   []string{"http://localhost:9200"},
		IndexPrefix: "deepwiki",
	}, zap.NewNop())
	if err != nil {
		t.Skipf("opensearch client: %v", err)
	}
	return c
}

func testChunks(repoID string) []model.Chunk {
	return []model.Chunk{
		{ChunkID: "chk_01J2X9K7QZ0ABCDEFGHJKMN1", RepoID: repoID, Path: "router/group.go", Content: "router group handles HTTP routing tree", Language: "go", StartLine: 1, EndLine: 10},
		{ChunkID: "chk_01J2X9K7QZ0ABCDEFGHJKMN2", RepoID: repoID, Path: "router/engine.go", Content: "engine dispatches requests to the router", Language: "go", StartLine: 1, EndLine: 20},
		{ChunkID: "chk_01J2X9K7QZ0ABCDEFGHJKMN3", RepoID: repoID, Path: "docs/guide.md", Content: "# Guide\nunrelated markdown documentation about deploy", Language: "markdown", StartLine: 1, EndLine: 5},
	}
}

func TestOpenSearchLifecycle(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	repoID := "repo_01J2X9K7QZ0ABCDEFGHJKMNQ"

	_ = c.DeleteIndex(ctx, repoID) // 清理残留

	if err := c.CreateIndex(ctx, repoID); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	exists, err := c.api.Indices.Exists(ctx, opensearchapi.IndicesExistsReq{Indices: []string{IndexName("deepwiki", repoID)}})
	if err != nil || exists.IsError() {
		t.Fatalf("index not visible after create: err=%v", err)
	}
	if err := c.CreateIndex(ctx, repoID); err != nil { // 幂等
		t.Fatalf("CreateIndex idempotent: %v", err)
	}

	chunks := testChunks(repoID)
	if err := c.BulkIndex(ctx, repoID, chunks); err != nil {
		t.Fatalf("BulkIndex: %v", err)
	}
	if _, err := c.api.Indices.Refresh(ctx, &opensearchapi.IndicesRefreshReq{Indices: []string{IndexName("deepwiki", repoID)}}); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	n, err := c.Count(ctx, repoID)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != int64(len(chunks)) {
		t.Fatalf("Count = %d, want %d", n, len(chunks))
	}

	hits, err := c.Search(ctx, repoID, "router", 10, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search router: no hits")
	}
	foundRouter := false
	for _, h := range hits {
		if h.Score <= 0 {
			t.Fatalf("hit %s score = %f, want positive BM25", h.ChunkID, h.Score)
		}
		for _, ch := range chunks {
			if ch.ChunkID == h.ChunkID && strings.Contains(ch.Content, "router") {
				foundRouter = true
			}
		}
	}
	if !foundRouter {
		t.Fatalf("no router-containing chunk in hits: %+v", hits)
	}

	pathHits, err := c.Search(ctx, repoID, "router", 10, "router/")
	if err != nil {
		t.Fatalf("Search pathFilter: %v", err)
	}
	for _, h := range pathHits {
		if h.ChunkID == "chk_01J2X9K7QZ0ABCDEFGHJKMN3" {
			t.Fatalf("pathFilter leaked docs/guide.md hit: %+v", h)
		}
	}

	miss, err := c.Search(ctx, repoID, "zzzzqqqq_nonexistent_term", 10, "")
	if err != nil {
		t.Fatalf("Search miss: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("Search miss got %d hits, want 0", len(miss))
	}

	if err := c.DeleteIndex(ctx, repoID); err != nil {
		t.Fatalf("DeleteIndex: %v", err)
	}
	n, err = c.Count(ctx, repoID)
	if err != nil {
		t.Fatalf("Count after delete: %v", err)
	}
	if n != 0 {
		t.Fatalf("Count after delete = %d, want 0", n)
	}
	if err := c.DeleteIndex(ctx, repoID); err != nil { // 不存在视为成功
		t.Fatalf("DeleteIndex idempotent: %v", err)
	}
}
