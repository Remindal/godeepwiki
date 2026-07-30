package retriever

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

type mqFakeInner struct {
	byQuery map[string][]model.ChunkHit
	err     error
}

func (f *mqFakeInner) Mode() string { return "hybrid" }
func (f *mqFakeInner) Search(ctx context.Context, repoID, query string, topK int) ([]model.ChunkHit, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byQuery[query], nil
}

type mqFakeLLM struct {
	content string
	err     error
}

func (f *mqFakeLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	if f.err != nil {
		return model.ChatResponse{}, f.err
	}
	return model.ChatResponse{Content: f.content}, nil
}
func (f *mqFakeLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	ch := make(chan model.StreamChunk)
	close(ch)
	return ch, nil
}
func (f *mqFakeLLM) ProviderName() string { return "fake" }
func (f *mqFakeLLM) ModelName() string    { return "fake" }

func TestMultiQuery_MergeAndBonus(t *testing.T) {
	inner := &mqFakeInner{byQuery: map[string][]model.ChunkHit{
		"原始问题": {
			{Chunk: model.Chunk{ChunkID: "chk_a", Path: "a.go"}, Score: 1.0},
			{Chunk: model.Chunk{ChunkID: "chk_b", Path: "b.go"}, Score: 0.5},
		},
		"变体一": {
			{Chunk: model.Chunk{ChunkID: "chk_a", Path: "a.go"}, Score: 1.0}, // 双路命中应加分
			{Chunk: model.Chunk{ChunkID: "chk_c", Path: "c.go"}, Score: 0.6},
		},
		"变体二": {
			{Chunk: model.Chunk{ChunkID: "chk_d", Path: "d.go"}, Score: 0.4},
		},
	}}
	l := &mqFakeLLM{content: "变体一\n变体二\n变体二\n"} // 含重复行应去重
	r := NewMultiQueryRetriever(inner, l, zap.NewNop())

	hits, err := r.Search(context.Background(), "repo_01J2X9K7QZ0ABCDEFGHJKMNP", "原始问题", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 4 {
		t.Fatalf("want 4 merged hits got %d: %+v", len(hits), hits)
	}
	// chk_a = 1.0 + 1.0*0.3 = 1.3 应排第一。
	if hits[0].Chunk.ChunkID != "chk_a" {
		t.Fatalf("chk_a should rank first: %+v", hits)
	}
	want := 1.3
	if hits[0].Score < want*0.99 || hits[0].Score > want*1.01 {
		t.Fatalf("chk_a score want ~%.2f got %.3f", want, hits[0].Score)
	}
}

func TestMultiQuery_RewriteFallback(t *testing.T) {
	inner := &mqFakeInner{byQuery: map[string][]model.ChunkHit{
		"原始问题": {{Chunk: model.Chunk{ChunkID: "chk_a"}, Score: 1.0}},
	}}
	l := &mqFakeLLM{err: errors.New("llm down")}
	r := NewMultiQueryRetriever(inner, l, zap.NewNop())

	hits, err := r.Search(context.Background(), "repo_01J2X9K7QZ0ABCDEFGHJKMNP", "原始问题", 10)
	if err != nil {
		t.Fatalf("fallback search: %v", err)
	}
	if len(hits) != 1 || hits[0].Chunk.ChunkID != "chk_a" {
		t.Fatalf("fallback should return original query hits: %+v", hits)
	}
}

func TestParseQueryLines(t *testing.T) {
	out := parseQueryLines("1. 查询一\n- 查询二\n\"查询三\"\n查询二\n\n", 3)
	if len(out) != 3 || out[0] != "查询一" || out[1] != "查询二" || out[2] != "查询三" {
		t.Fatalf("parseQueryLines: %+v", out)
	}
}
