package ingest

import (
	"context"
	"strings"
	"testing"
)

func TestChunkFilesSmallFile(t *testing.T) {
	files := []SourceFile{{Path: "a.go", Language: "go", Content: "line1\nline2\nline3", Hash: "h1"}}
	chunks, err := ChunkFiles(context.Background(), "repo_1", files, IngestOptions{ChunkSize: 500, ChunkOverlap: 100})
	if err != nil {
		t.Fatalf("ChunkFiles: %v", err)
	}
	// 父子块双层索引：小文件 → 1 父块 + 1 子块。
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (1 parent + 1 child)", len(chunks))
	}
	c := chunks[0]
	if c.StartLine != 1 || c.EndLine != 3 {
		t.Fatalf("span = %d-%d, want 1-3", c.StartLine, c.EndLine)
	}
	if c.Content != "line1\nline2\nline3" {
		t.Fatalf("content = %q", c.Content)
	}
	if !strings.HasPrefix(c.ChunkID, "chk_") {
		t.Fatalf("chunk id = %q", c.ChunkID)
	}
	if c.RepoID != "repo_1" || c.Path != "a.go" || c.Language != "go" || c.FileHash != "h1" {
		t.Fatalf("unexpected chunk fields: %+v", c)
	}
	if c.ParentChunkID != "" {
		t.Fatalf("parent chunk should have empty ParentChunkID, got %q", c.ParentChunkID)
	}
	child := chunks[1]
	if child.ParentChunkID != c.ChunkID {
		t.Fatalf("child ParentChunkID = %q, want parent %q", child.ParentChunkID, c.ChunkID)
	}
	if child.StartLine != 1 || child.EndLine != 3 || child.Content != c.Content {
		t.Fatalf("unexpected child span/content: %+v", child)
	}
}

func TestChunkFilesWindowOverlap(t *testing.T) {
	// 每行 40 字符 ≈ 10 token；chunkSize=50 → 每块 5 行；overlap=10 → 回退 1 行。
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		sb.WriteString(strings.Repeat("x", 40))
		sb.WriteString("\n")
	}
	content := strings.TrimRight(sb.String(), "\n")
	files := []SourceFile{{Path: "big.go", Language: "go", Content: content, Hash: "h"}}
	chunks, err := ChunkFiles(context.Background(), "r", files, IngestOptions{ChunkSize: 50, ChunkOverlap: 10})
	if err != nil {
		t.Fatalf("ChunkFiles: %v", err)
	}
	if len(chunks) < 4 {
		t.Fatalf("got %d chunks, want >= 4", len(chunks))
	}
	for i, c := range chunks {
		if c.StartLine < 1 || c.EndLine > 20 || c.StartLine > c.EndLine {
			t.Fatalf("chunk %d invalid span %d-%d", i, c.StartLine, c.EndLine)
		}
		if i > 0 && chunks[i].StartLine > chunks[i-1].EndLine {
			t.Fatalf("chunk %d start %d should overlap previous end %d", i, chunks[i].StartLine, chunks[i-1].EndLine)
		}
	}
	if chunks[len(chunks)-1].EndLine != 20 {
		t.Fatalf("last chunk end = %d, want 20", chunks[len(chunks)-1].EndLine)
	}
}

func TestChunkFilesMarkdownHeadings(t *testing.T) {
	content := "# Intro\nline a\nline b\n# Usage\nline c\nline d"
	files := []SourceFile{{Path: "README.md", Language: "markdown", Content: content, Hash: "h"}}
	chunks, err := ChunkFiles(context.Background(), "r", files, IngestOptions{ChunkSize: 500, ChunkOverlap: 100})
	if err != nil {
		t.Fatalf("ChunkFiles: %v", err)
	}
	// 标题强制切 2 个父块，各带 1 个子块 → 共 4 块。
	if len(chunks) != 4 {
		t.Fatalf("got %d chunks, want 4 (2 parents + 2 children)", len(chunks))
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 3 {
		t.Fatalf("parent0 span = %d-%d, want 1-3", chunks[0].StartLine, chunks[0].EndLine)
	}
	if chunks[2].StartLine != 4 || chunks[2].EndLine != 6 {
		t.Fatalf("parent1 span = %d-%d, want 4-6", chunks[2].StartLine, chunks[2].EndLine)
	}
	if !strings.HasPrefix(chunks[2].Content, "# Usage") {
		t.Fatalf("parent1 content = %q", chunks[2].Content)
	}
	if chunks[1].ParentChunkID != chunks[0].ChunkID || chunks[3].ParentChunkID != chunks[2].ChunkID {
		t.Fatalf("children not linked to parents: %+v", chunks)
	}
}

func TestChunkFilesEmptyAndCancel(t *testing.T) {
	files := []SourceFile{{Path: "empty.go", Language: "go", Content: "", Hash: "h"}}
	chunks, err := ChunkFiles(context.Background(), "r", files, IngestOptions{})
	if err != nil {
		t.Fatalf("ChunkFiles: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("got %d chunks, want 0", len(chunks))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ChunkFiles(ctx, "r", files, IngestOptions{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("abcd"); got != 1 {
		t.Fatalf("EstimateTokens(abcd) = %d, want 1", got)
	}
	if got := EstimateTokens("abcde"); got != 2 {
		t.Fatalf("EstimateTokens(abcde) = %d, want 2", got)
	}
}
