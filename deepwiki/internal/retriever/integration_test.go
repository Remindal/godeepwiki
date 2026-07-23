package retriever

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
)

const itRepoID = "repo_01J2X9K7QZ0ABCDEFGHJKMNQRS"

func itChunkIDs() []string {
	return []string{
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQSA",
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQSB",
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQSC",
	}
}

// itSetup 准备真实 Postgres（chunks 表）与 OpenSearch（索引）测试数据；任一不可达则 Skip。
func itSetup(t *testing.T) (context.Context, *pgxpool.Pool, *search.Client, store.ChunkStore) {
	t.Helper()
	ctx := context.Background()

	pgConn, err := net.DialTimeout("tcp", "localhost:5432", 2*time.Second)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	_ = pgConn.Close()
	osConn, err := net.DialTimeout("tcp", "localhost:9200", 2*time.Second)
	if err != nil {
		t.Skipf("opensearch unreachable: %v", err)
	}
	_ = osConn.Close()

	db, err := store.Open(ctx, "postgres://deepwiki:deepwiki@localhost:5432/deepwiki", 5, zap.NewNop())
	if err != nil {
		t.Skipf("postgres open: %v", err)
	}
	t.Cleanup(db.Close)
	pool := db.Pool()
	chunks := store.NewChunkStore(db, zap.NewNop())

	osCli, err := search.NewClient(ctx, config.OpenSearchConfig{
		Addresses:   []string{"http://localhost:9200"},
		IndexPrefix: "deepwiki",
	}, zap.NewNop())
	if err != nil {
		t.Skipf("opensearch client: %v", err)
	}

	// 清理残留 + 准备 repo/chunks 行
	_ = osCli.DeleteIndex(ctx, itRepoID)
	_, _ = pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id = $1`, itRepoID)
	_, _ = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id = $1`, itRepoID)
	if _, err := pool.Exec(ctx, `INSERT INTO repos (repo_id, repo_url, branch, created_at, updated_at) VALUES ($1, 'https://github.com/it/test', 'main', now(), now())`, itRepoID); err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM chunks WHERE repo_id = $1`, itRepoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM repos WHERE repo_id = $1`, itRepoID)
		_ = osCli.DeleteIndex(context.Background(), itRepoID)
	})

	ids := itChunkIDs()
	rows := []struct {
		path, content, lang string
		vec                 []float32
	}{
		{"router/group.go", "router group handles HTTP routing tree registration", "go", itVector(1536, 0, 1.0)},
		{"router/engine.go", "engine dispatches requests to router middleware chain", "go", itVector1536(0, 0.9, 1, 0.1)},
		{"docs/deploy.md", "deployment guide for production clusters", "markdown", itVector(1536, 2, 1.0)},
	}
	for i, r := range rows {
		if _, err := pool.Exec(ctx, `
			INSERT INTO chunks (chunk_id, repo_id, path, start_line, end_line, language, content, file_hash, embedding_model, embedding)
			VALUES ($1, $2, $3, 1, 10, $4, $5, 'hash', 'it-model', $6)
		`, ids[i], itRepoID, r.path, r.lang, r.content, pgvector.NewVector(r.vec)); err != nil {
			t.Fatalf("insert chunk %s: %v", ids[i], err)
		}
	}

	if err := osCli.CreateIndex(ctx, itRepoID); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	docs := []model.Chunk{
		{ChunkID: ids[0], RepoID: itRepoID, Path: rows[0].path, Content: rows[0].content, Language: rows[0].lang, StartLine: 1, EndLine: 10},
		{ChunkID: ids[1], RepoID: itRepoID, Path: rows[1].path, Content: rows[1].content, Language: rows[1].lang, StartLine: 1, EndLine: 10},
		{ChunkID: ids[2], RepoID: itRepoID, Path: rows[2].path, Content: rows[2].content, Language: rows[2].lang, StartLine: 1, EndLine: 10},
	}
	if err := osCli.BulkIndex(ctx, itRepoID, docs); err != nil {
		t.Fatalf("BulkIndex: %v", err)
	}
	if err := osCli.Refresh(ctx, itRepoID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return ctx, pool, osCli, chunks
}

// itVector 1536 维 one-hot 向量（idx 位置为 val）。
func itVector(dim, idx int, val float32) []float32 {
	v := make([]float32, dim)
	v[idx] = val
	return v
}

// itVector1536 指定两个分量的 1536 维向量。
func itVector1536(i1 int, v1 float32, i2 int, v2 float32) []float32 {
	v := make([]float32, 1536)
	v[i1], v[i2] = v1, v2
	return v
}

// fakeEmbedder 固定返回查询向量 [1,0,0,...]。
type fakeEmbedder struct{ vec []float32 }

func (f fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = f.vec
	}
	return out, nil
}

func (f fakeEmbedder) Dimensions() int      { return len(f.vec) }
func (f fakeEmbedder) ProviderName() string { return "fake" }
func (f fakeEmbedder) ModelName() string    { return "fake-embed" }

func TestKeywordRetrieverIntegration(t *testing.T) {
	ctx, _, osCli, chunks := itSetup(t)
	r := NewKeywordRetriever(osCli, chunks, zap.NewNop())

	hits, err := r.Search(ctx, itRepoID, "router", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits for router")
	}
	for _, h := range hits {
		if h.Score <= 0 {
			t.Fatalf("hit %s BM25 score = %f, want positive", h.Chunk.ChunkID, h.Score)
		}
		if !strings.Contains(h.Chunk.Content, "router") {
			t.Fatalf("hit %s content not backfilled or mismatched: %q", h.Chunk.ChunkID, h.Chunk.Content)
		}
		if h.Chunk.Path == "" || h.Chunk.StartLine < 1 {
			t.Fatalf("hit %s chunk fields incomplete: %+v", h.Chunk.ChunkID, h.Chunk)
		}
	}

	filtered, err := r.SearchWithPath(ctx, itRepoID, "router", 10, "router/")
	if err != nil {
		t.Fatalf("SearchWithPath: %v", err)
	}
	for _, h := range filtered {
		if !strings.HasPrefix(h.Chunk.Path, "router/") {
			t.Fatalf("pathFilter leaked %s", h.Chunk.Path)
		}
	}
}

func TestVectorRetrieverIntegration(t *testing.T) {
	ctx, pool, _, _ := itSetup(t)
	r := NewVectorRetriever(pool, fakeEmbedder{vec: itVector(1536, 0, 1.0)}, 64, zap.NewNop())

	hits, err := r.Search(ctx, itRepoID, "anything", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3", len(hits))
	}
	ids := itChunkIDs()
	if hits[0].Chunk.ChunkID != ids[0] {
		t.Fatalf("nearest = %s, want %s (one-hot [1,0,...])", hits[0].Chunk.ChunkID, ids[0])
	}
	for i := 1; i < len(hits); i++ {
		if hits[i-1].Score < hits[i].Score {
			t.Fatalf("scores not descending: %+v", hits)
		}
	}
	if hits[0].Score < 0.99 {
		t.Fatalf("exact vector score = %f, want ~1.0", hits[0].Score)
	}
}

func TestHybridRetrieverIntegration(t *testing.T) {
	ctx, pool, osCli, chunks := itSetup(t)
	kw := NewKeywordRetriever(osCli, chunks, zap.NewNop())
	vec := NewVectorRetriever(pool, fakeEmbedder{vec: itVector(1536, 0, 1.0)}, 64, zap.NewNop())
	r := NewHybridRetriever(kw, vec, 60, zap.NewNop())

	hits, err := r.Search(ctx, itRepoID, "router", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hybrid hits")
	}
	seen := make(map[string]bool, len(hits))
	for _, h := range hits {
		if seen[h.Chunk.ChunkID] {
			t.Fatalf("duplicate chunk_id %s in hybrid results", h.Chunk.ChunkID)
		}
		seen[h.Chunk.ChunkID] = true
		if h.Score <= 0 {
			t.Fatalf("hit %s RRF score = %f, want positive", h.Chunk.ChunkID, h.Score)
		}
	}
	for i := 1; i < len(hits); i++ {
		if hits[i-1].Score < hits[i].Score {
			t.Fatalf("RRF scores not descending: %+v", hits)
		}
	}
}
