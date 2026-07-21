package ingest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestRealGinParseAndChunk 真实浅克隆 gin-gonic/gin，验证 parser+chunker 全链路；
// 网络不可达时 Skip（环境限制，不影响单元验收）。
func TestRealGinParseAndChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	root := t.TempDir()
	dest := filepath.Join(root, "gin")

	cloner := NewGitCloner("git", 3*time.Minute, zap.NewNop())
	ctx := context.Background()
	if err := cloner.Clone(ctx, "https://github.com/gin-gonic/gin", "master", dest); err != nil {
		t.Skipf("github unreachable, skip real-clone acceptance: %v", err)
	}

	files, err := ParseFiles(ctx, dest, IngestOptions{
		IncludeExt:  []string{".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".java", ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".rs", ".cpp", ".c", ".h", ".rb", ".php", ".sh", ".sql", ".html", ".css"},
		ExcludeDirs: []string{".git", "node_modules", "vendor", "dist", "build", "target"},
	})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed from gin repo")
	}
	for _, f := range files {
		if strings.HasPrefix(f.Path, "vendor/") || strings.Contains(f.Path, "/vendor/") ||
			strings.HasPrefix(f.Path, "node_modules/") || strings.Contains(f.Path, "/node_modules/") {
			t.Fatalf("skipped dir leaked into results: %s", f.Path)
		}
	}
	t.Logf("parsed %d files", len(files))

	chunks, err := ChunkFiles(ctx, "repo_gin", files, IngestOptions{ChunkSize: 500, ChunkOverlap: 100})
	if err != nil {
		t.Fatalf("ChunkFiles: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("no chunks produced")
	}
	seenIDs := make(map[string]struct{}, len(chunks))
	for _, c := range chunks {
		if c.Path == "" || c.StartLine < 1 || c.StartLine > c.EndLine || !strings.HasPrefix(c.ChunkID, "chk_") {
			t.Fatalf("invalid chunk: %+v", c)
		}
		if _, dup := seenIDs[c.ChunkID]; dup {
			t.Fatalf("duplicate chunk id %s", c.ChunkID)
		}
		seenIDs[c.ChunkID] = struct{}{}
		if len(c.FileHash) != 16 {
			t.Fatalf("chunk %s file hash len = %d", c.ChunkID, len(c.FileHash))
		}
	}
	t.Logf("produced %d chunks from %d files", len(chunks), len(files))
}
