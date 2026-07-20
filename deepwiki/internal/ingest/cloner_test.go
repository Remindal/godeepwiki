package ingest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestFetchAndResetLocal 在本地 bare 仓库上验证 fetch → reset → clean 能更新到最新 commit。
func TestFetchAndResetLocal(t *testing.T) {
	tmpDir := t.TempDir()
	bare := filepath.Join(tmpDir, "origin.git")
	cloneDir := filepath.Join(tmpDir, "repo")

	// 初始化 bare 仓库并推送一个文件
	mustExec(t, "git", "init", "--bare", "--initial-branch=main", bare)
	work := filepath.Join(tmpDir, "work")
	mustExec(t, "git", "clone", bare, work)
	writeFile(t, filepath.Join(work, "a.txt"), "v1")
	mustExecInDir(t, work, "git", "add", "a.txt")
	mustExecInDir(t, work, "git", "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "v1")
	mustExecInDir(t, work, "git", "push", "origin", "main")

	// 浅克隆
	logger, _ := zap.NewDevelopment()
	cloner := NewGitCloner("git", 2*time.Minute, logger)
	ctx := context.Background()
	if err := cloner.Clone(ctx, bare, "main", cloneDir); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// 在 bare 上新增 commit
	writeFile(t, filepath.Join(work, "b.txt"), "v2")
	mustExecInDir(t, work, "git", "add", "b.txt")
	mustExecInDir(t, work, "git", "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "v2")
	mustExecInDir(t, work, "git", "push", "origin", "main")

	newHash, err := cloner.FetchAndReset(ctx, cloneDir, "main")
	if err != nil {
		t.Fatalf("fetch/reset failed: %v", err)
	}
	if len(newHash) != 40 {
		t.Fatalf("expected 40-char hash, got %q", newHash)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "b.txt")); err != nil {
		t.Fatalf("expected b.txt after fetch/reset: %v", err)
	}
}

func TestIgnoreDefaults(t *testing.T) {
	filter := NewFileFilter(nil, nil, 0)
	for _, d := range []string{".git", "vendor", "node_modules"} {
		if !filter.SkipDir(d) {
			t.Fatalf("expected SkipDir(%s)=true", d)
		}
	}
	if !filter.SkipFile("node_modules/foo.js", 100, false) {
		t.Fatal("expected DefaultSkipDirs to skip node_modules file")
	}
	if !filter.SkipFile("big.bin", DefaultMaxFileSize+1, false) {
		t.Fatal("expected max file size skip")
	}
	if !filter.SkipFile("../etc/passwd", 100, false) {
		t.Fatal("expected path traversal skip")
	}
	if filter.SkipFile("main.go", 100, false) {
		t.Fatal("did not expect main.go to be skipped")
	}
}

func mustExec(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, string(out))
	}
}

func mustExecInDir(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v in %s: %v\n%s", name, args, dir, err, string(out))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
