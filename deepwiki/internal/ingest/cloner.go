// Package ingest 仓库摄取：克隆、解析、切分与 Pipeline 编排。
package ingest

import (
	"context"
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
func (c *GitCloner) Clone(ctx context.Context, url, branch, destDir string) error {
	// TODO: 实现浅克隆，要求（硬约束 #5，禁止 git pull / 禁止 sh -c）：
	// ① exec.CommandContext(ctx, c.binaryPath, "clone", "--depth", "1", "--single-branch",
	//    "--branch", branch, url, destDir)：参数必须以独立数组元素传递，禁止拼成单个字符串经 sh -c 执行；
	// ② cmd.Env 在 os.Environ() 上追加 GIT_TERMINAL_PROMPT=0，杜绝交互式凭据提示卡死 worker；
	// ③ 单次操作挂 context.WithTimeout(ctx, c.opTimeout)（git.op_timeout，默认 10m）；
	// ④ branch 为空时省略 --branch（远端默认分支）；
	// ⑤ 失败返回 fmt.Errorf 包装；stderr 截断进 zap 日志，禁止向客户端回传原始输出（硬约束 #8）。
	panic("TODO: GitCloner.Clone not implemented")
}

// FetchAndReset 等价于 git fetch → reset --hard FETCH_HEAD → clean -fdx。
// 禁止 git pull：工作区脏状态与 merge 冲突会直接卡死 pipeline（硬约束 #5）。
func (c *GitCloner) FetchAndReset(ctx context.Context, repoDir, branch string) (string, error) {
	// TODO: 依次执行三条命令（均为 exec.CommandContext 独立调用，禁止 git pull，禁止 sh -c）：
	// ① git -C <repoDir> fetch --depth 1 origin <branch>
	// ② git -C <repoDir> reset --hard FETCH_HEAD（reset 目标必须是 FETCH_HEAD，fetch 后即取，避免并发漂移）
	// ③ git -C <repoDir> clean -fdx（清理未跟踪文件）
	// 每步 env 带 GIT_TERMINAL_PROMPT=0，各挂 context.WithTimeout(c.opTimeout)；
	// 任一步失败返回错误，由调用方回退为「重新 clone 到 ./data/repos/.tmp/<task_id>/ 后 os.Rename 原子切换」；
	// 返回新 commit hash：git -C <repoDir> rev-parse HEAD 输出 strings.TrimSpace。
	panic("TODO: GitCloner.FetchAndReset not implemented")
}

// LsRemote 取远端分支 HEAD commit（ingest 幂等判断用，基线 §6.1；轻量、不落盘）。
// 失败（网络/私有仓库无凭据）时由调用方放行创建任务，clone 阶段再报错。
func (c *GitCloner) LsRemote(ctx context.Context, url, branch string) (string, error) {
	// TODO: exec.CommandContext(ctx, c.binaryPath, "ls-remote", url, "refs/heads/"+branch)，
	// 解析首列 hash；branch 为空时改查 "HEAD"；env GIT_TERMINAL_PROMPT=0 + context.WithTimeout(c.opTimeout)；
	// 未匹配到分支返回 fmt.Errorf("branch %q not found on remote %s", branch, url)。
	panic("TODO: GitCloner.LsRemote not implemented")
}
