package retriever

import (
	"testing"

	"deepwiki/internal/model"
)

// TestRRFFuse_LangWeight 验证 docs 类 chunk 在 RRF 融合中被降权：
// 同分情况下代码 chunk 必须排在 markdown chunk 前面。
func TestRRFFuse_LangWeight(t *testing.T) {
	code := model.ChunkHit{Chunk: model.Chunk{ChunkID: "chk_code", Path: "routergroup.go", Language: "go"}}
	doc := model.ChunkHit{Chunk: model.Chunk{ChunkID: "chk_doc", Path: "docs/doc.md", Language: "markdown"}}

	// 两路结果中 doc 都排第 1、code 都排第 2：无权重时 doc 在前的极端场景。
	kw := []model.ChunkHit{doc, code}
	vec := []model.ChunkHit{doc, code}

	out := rrfFuse(60, 2, kw, vec)
	if len(out) != 2 {
		t.Fatalf("want 2 hits got %d", len(out))
	}
	if out[0].Chunk.ChunkID != "chk_code" {
		t.Fatalf("code chunk should rank first after lang weight: %+v", out)
	}
	if out[0].Score <= out[1].Score {
		t.Fatalf("code score %.4f should > doc score %.4f", out[0].Score, out[1].Score)
	}
}

// TestRRFFuse_MergeByChunkID 验证同 chunk_id 多路命中分数累加。
func TestRRFFuse_MergeByChunkID(t *testing.T) {
	h := model.ChunkHit{Chunk: model.Chunk{ChunkID: "chk_same", Path: "a.go", Language: "go"}}
	out := rrfFuse(60, 5, []model.ChunkHit{h}, []model.ChunkHit{h})
	if len(out) != 1 {
		t.Fatalf("want 1 merged hit got %d", len(out))
	}
	want := 2.0 / 61.0 // 两路各 rank=1 → 2 * 1/(60+1)
	if out[0].Score < want*0.99 || out[0].Score > want*1.01 {
		t.Fatalf("merged score want ~%.5f got %.5f", want, out[0].Score)
	}
}
