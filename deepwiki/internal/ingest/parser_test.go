package ingest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "README.md"), "# Title\n")
	writeFile(t, filepath.Join(root, "script.py"), "print('hi')\n")
	writeFile(t, filepath.Join(root, "unknown.xyz"), "???\n")
	writeFile(t, filepath.Join(root, "bin.dat"), string([]byte{0x89, 0x50, 0x00, 0x47}))
	writeFile(t, filepath.Join(root, "big.txt"), strings.Repeat("a", int(DefaultMaxFileSize)+1))
	writeFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "module.exports = {}\n")
	writeFile(t, filepath.Join(root, "vendor", "v.go"), "package v\n")
	writeFile(t, filepath.Join(root, ".git", "config"), "[core]\n")

	opts := IngestOptions{IncludeExt: []string{".go", ".py", ".md", ".txt", ".dat", ".xyz"}}
	files, err := ParseFiles(context.Background(), root, opts)
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}

	byPath := make(map[string]SourceFile, len(files))
	for _, f := range files {
		byPath[f.Path] = f
		if len(f.Hash) != 16 {
			t.Fatalf("file %s hash len = %d, want 16", f.Path, len(f.Hash))
		}
	}
	if len(files) != 4 {
		t.Fatalf("got %d files (%v), want 4", len(files), keys(byPath))
	}
	if byPath["unknown.xyz"].Language != "" {
		t.Fatalf("unknown.xyz language = %q, want empty", byPath["unknown.xyz"].Language)
	}
	if byPath["main.go"].Language != "go" {
		t.Fatalf("main.go language = %q", byPath["main.go"].Language)
	}
	if byPath["README.md"].Language != "markdown" {
		t.Fatalf("README.md language = %q", byPath["README.md"].Language)
	}
	if byPath["script.py"].Language != "python" {
		t.Fatalf("script.py language = %q", byPath["script.py"].Language)
	}
	for _, skipped := range []string{"node_modules/pkg/index.js", "vendor/v.go", ".git/config", "bin.dat", "big.txt"} {
		if _, ok := byPath[skipped]; ok {
			t.Fatalf("expected %s to be skipped", skipped)
		}
	}
}

func TestParseFilesIncludeExtFilter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "README.md"), "# Title\n")

	files, err := ParseFiles(context.Background(), root, IngestOptions{IncludeExt: []string{".go"}})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if len(files) != 1 || files[0].Path != "main.go" {
		t.Fatalf("got %v, want only main.go", files)
	}
}

func TestParseFilesCancel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ParseFiles(ctx, root, IngestOptions{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestParseFilesUnknownExtKeepsEmptyLanguage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "data.xyz"), "plain text content\n")
	files, err := ParseFiles(context.Background(), root, IngestOptions{})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if len(files) != 1 || files[0].Language != "" {
		t.Fatalf("got %+v, want 1 file with empty language", files)
	}
}

func keys(m map[string]SourceFile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
