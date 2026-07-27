// Package ingest 仓库摄取：克隆、解析、切分与 Pipeline 编排。
package ingest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Cloner Git 抽象（基线 §7，冻结签名）。
type Cloner interface {
	// Clone 克隆到 destDir（调用方负责传入 .tmp 临时目录，成功后原子 rename）。
	Clone(ctx context.Context, url, branch, destDir string) error
	// FetchAndReset = git fetch --depth 1 origin <branch> → git reset --hard FETCH_HEAD → git clean -fdx。
	// 禁止 git pull（硬约束 #5）；返回新的 commit hash。
	FetchAndReset(ctx context.Context, repoDir, branch string) (newCommitHash string, err error)
	// LsRemote 取远端分支 HEAD commit（ingest 幂等判断用，基线 §6.1；轻量、不落盘）。
	LsRemote(ctx context.Context, url, branch string) (string, error)
}

// GitCloner 基于系统 git CLI 的实现（exec.CommandContext，无 shell；CommandContext 天然支持 ctx 取消）。
type GitCloner struct {
	binaryPath string        // git.binary_path，默认 "git"
	opTimeout  time.Duration // git.op_timeout，默认 10m
	logger     *zap.Logger
}

// NewGitCloner binaryPath 为空取 "git"；opTimeout ≤ 0 取 10 分钟。
func NewGitCloner(binaryPath string, opTimeout time.Duration, logger *zap.Logger) *GitCloner {
	if binaryPath == "" {
		binaryPath = "git"
	}
	if opTimeout <= 0 {
		opTimeout = 10 * time.Minute
	}
	return &GitCloner{binaryPath: binaryPath, opTimeout: opTimeout, logger: logger}
}

var _ Cloner = (*GitCloner)(nil)

// Clone 浅克隆指定分支到 destDir；branch 为空时取远端默认分支。
// git clone 失败（GFW 指纹封锁/代理不稳定）自动降级 tarball 下载（Go net/http，TLS 指纹可用）。
func (c *GitCloner) Clone(ctx context.Context, url, branch, destDir string) error {
	// 硬约束 #5：禁止 git pull；硬约束 #11：参数必须独立数组元素，禁止 sh -c；路径经 filepath.Clean 防穿越。
	destDir, err := validatePath(destDir)
	if err != nil {
		return err
	}
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, url, destDir)

	_, stderr, err := c.run(ctx, c.cloneTimeout(), args...)
	if err != nil {
		c.logger.Warn("git clone failed, fallback to tarball",
			zap.String("url", url), zap.String("branch", branch), zap.String("dest", destDir),
			zap.String("stderr", truncate(stderr, 512)), zap.Error(err))
		if tbErr := c.cloneViaTarball(ctx, url, branch, destDir); tbErr != nil {
			c.logger.Error("git clone failed (tarball fallback also failed)", zap.String("url", url), zap.Error(tbErr))
			return fmt.Errorf("git clone: %w", err)
		}
	}
	return nil
}

// FetchAndReset 等价于 git fetch → reset --hard FETCH_HEAD → clean -fdx。
// 禁止 git pull：工作区脏状态与 merge 冲突会直接卡死 pipeline（硬约束 #5）。
func (c *GitCloner) FetchAndReset(ctx context.Context, repoDir, branch string) (string, error) {
	repoDir, err := validatePath(repoDir)
	if err != nil {
		return "", err
	}
	timeout := c.fetchTimeout()

	steps := [][]string{
		{"-C", repoDir, "fetch", "--depth", "1", "origin", branch},
		{"-C", repoDir, "reset", "--hard", "FETCH_HEAD"},
		{"-C", repoDir, "clean", "-fdx"},
	}
	for _, args := range steps {
		_, stderr, err := c.run(ctx, timeout, args...)
		if err != nil {
			c.logger.Error("git fetch/reset/clean failed", zap.Strings("args", args), zap.String("stderr", truncate(stderr, 1024)), zap.Error(err))
			return "", fmt.Errorf("git %s: %w", args[2], err)
		}
	}

	hash, stderr, err := c.run(ctx, timeout, "-C", repoDir, "rev-parse", "HEAD")
	if err != nil {
		c.logger.Error("git rev-parse failed", zap.String("stderr", truncate(stderr, 1024)), zap.Error(err))
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(hash), nil
}

// LsRemote 取远端分支 HEAD commit（ingest 幂等判断用，基线 §6.1；轻量、不落盘）。
// 失败（网络/私有仓库无凭据）时由调用方放行创建任务，clone 阶段再报错。
func (c *GitCloner) LsRemote(ctx context.Context, url, branch string) (string, error) {
	ref := "HEAD"
	if branch != "" {
		ref = "refs/heads/" + branch
	}
	stdout, stderr, err := c.run(ctx, c.lsRemoteTimeout(), "ls-remote", url, ref)
	if err != nil {
		c.logger.Error("git ls-remote failed", zap.String("url", url), zap.String("ref", ref), zap.String("stderr", truncate(stderr, 1024)), zap.Error(err))
		return "", fmt.Errorf("git ls-remote: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 && len(fields[0]) >= 7 {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("branch %q not found on remote %s", branch, url)
}

// run 执行 git 命令，返回 stdout、stderr、error；env 追加 GIT_TERMINAL_PROMPT=0。
func (c *GitCloner) run(ctx context.Context, timeout time.Duration, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func (c *GitCloner) cloneTimeout() time.Duration {
	return c.opTimeout
}

func (c *GitCloner) fetchTimeout() time.Duration {
	if c.opTimeout < 5*time.Minute {
		return c.opTimeout
	}
	return 5 * time.Minute
}

func (c *GitCloner) lsRemoteTimeout() time.Duration {
	if c.opTimeout < 30*time.Second {
		return c.opTimeout
	}
	return 30 * time.Second
}

// validatePath 防止路径穿越（硬约束 #11）：filepath.Clean 后若以 ".." 开头则拒绝。
func validatePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: %q", p)
	}
	return clean, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
