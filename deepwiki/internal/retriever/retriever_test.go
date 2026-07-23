package retriever

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

func hit(id string, score float64) model.ChunkHit {
	return model.ChunkHit{Chunk: model.Chunk{ChunkID: id}, Score: score}
}

func TestRRFFuse(t *testing.T) {
	kw := []model.ChunkHit{hit("a", 10), hit("b", 8), hit("c", 6)}
	vec := []model.ChunkHit{hit("b", 0.9), hit("d", 0.8), hit("a", 0.7)}

	out := rrfFuse(60, 10, kw, vec)
	ids := make(map[string]bool, len(out))
	for _, h := range out {
		if ids[h.Chunk.ChunkID] {
			t.Fatalf("duplicate chunk_id %s in fused results", h.Chunk.ChunkID)
		}
		ids[h.Chunk.ChunkID] = true
	}
	if len(out) != 4 {
		t.Fatalf("got %d fused hits, want 4 (a,b,c,d)", len(out))
	}
	// a: 1/61 + 1/63；b: 1/62 + 1/61；c: 1/63；d: 1/62 → b > a > d > c
	wantOrder := []string{"b", "a", "d", "c"}
	for i, id := range wantOrder {
		if out[i].Chunk.ChunkID != id {
			t.Fatalf("fused[%d] = %s, want %s (scores: %+v)", i, out[i].Chunk.ChunkID, id, out)
		}
	}
	for i := 1; i < len(out); i++ {
		if out[i-1].Score < out[i].Score {
			t.Fatalf("scores not descending: %+v", out)
		}
	}

	truncated := rrfFuse(60, 2, kw, vec)
	if len(truncated) != 2 {
		t.Fatalf("topK truncation got %d, want 2", len(truncated))
	}
}

// fakeRetriever 可编程的 inner Retriever。
type fakeRetriever struct {
	hits []model.ChunkHit
	err  error
	mode string
}

func (f fakeRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	if f.err != nil {
		return nil, f.err
	}
	return truncateHits(f.hits, topK), nil
}

func (f fakeRetriever) Mode() string { return f.mode }

type fakeReranker struct {
	out []model.ChunkHit
	err error
}

func (f fakeReranker) Rerank(ctx context.Context, query string, candidates []model.ChunkHit) ([]model.ChunkHit, error) {
	return f.out, f.err
}

func TestRerankRetriever(t *testing.T) {
	cands := []model.ChunkHit{hit("a", 1), hit("b", 2), hit("c", 3), hit("d", 4), hit("e", 5)}

	// 无 reranker：仅按 topK 截断，保持 inner 顺序。
	r := NewRerankRetriever(fakeRetriever{hits: cands, mode: "hybrid"}, zap.NewNop())
	out, err := r.Search(context.Background(), "repo_x", "q", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if r.Mode() != "hybrid" {
		t.Fatalf("Mode = %q, want inner mode hybrid", r.Mode())
	}
	if len(out) != 2 || out[0].Chunk.ChunkID != "a" || out[1].Chunk.ChunkID != "b" {
		t.Fatalf("passthrough truncation wrong: %+v", out)
	}

	// reranker 重排：按重排结果截断。
	reversed := []model.ChunkHit{hit("e", 10), hit("d", 9), hit("c", 8), hit("b", 7), hit("a", 6)}
	r2 := NewRerankRetriever(fakeRetriever{hits: cands}, zap.NewNop()).WithReranker(fakeReranker{out: reversed})
	out2, err := r2.Search(context.Background(), "repo_x", "q", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(out2) != 2 || out2[0].Chunk.ChunkID != "e" || out2[1].Chunk.ChunkID != "d" {
		t.Fatalf("rerank truncation wrong: %+v", out2)
	}

	// reranker 失败：降级 inner 原序。
	r3 := NewRerankRetriever(fakeRetriever{hits: cands}, zap.NewNop()).WithReranker(fakeReranker{err: errors.New("llm down")})
	out3, err := r3.Search(context.Background(), "repo_x", "q", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(out3) != 2 || out3[0].Chunk.ChunkID != "a" {
		t.Fatalf("fallback to inner order wrong: %+v", out3)
	}

	// inner 失败：错误透传。
	r4 := NewRerankRetriever(fakeRetriever{err: errors.New("inner down")}, zap.NewNop())
	if _, err := r4.Search(context.Background(), "repo_x", "q", 2); err == nil {
		t.Fatal("expected inner error passthrough")
	}
}

func TestParseScoreArray(t *testing.T) {
	scores, err := parseScoreArray("```json\n[9, 3.5, 7]\n```", 3)
	if err != nil {
		t.Fatalf("parseScoreArray: %v", err)
	}
	if len(scores) != 3 || scores[0] != 9 || scores[1] != 3.5 || scores[2] != 7 {
		t.Fatalf("scores = %v", scores)
	}
	if _, err := parseScoreArray("no array here", 2); err == nil {
		t.Fatal("expected error for missing array")
	}
	if _, err := parseScoreArray("[1, 2]", 3); err == nil {
		t.Fatal("expected error for count mismatch")
	}
}

func TestKeywordRetrieverInvalidRepoID(t *testing.T) {
	r := NewKeywordRetriever(nil, nil, zap.NewNop())
	if _, err := r.Search(context.Background(), "bad-id", "q", 5); err == nil {
		t.Fatal("expected invalid repo_id error")
	}
}

func TestVectorRetrieverInvalidRepoID(t *testing.T) {
	r := NewVectorRetriever(nil, nil, 64, zap.NewNop())
	if _, err := r.SearchByVector(context.Background(), "../etc", []float32{1}, 5, ""); err == nil {
		t.Fatal("expected invalid repo_id error")
	}
	if _, err := r.SearchByVector(context.Background(), "repo_01J2X9K7QZ0ABCDEFGHJKMNQRS", nil, 5, ""); err == nil {
		t.Fatal("expected empty vector error")
	}
}
