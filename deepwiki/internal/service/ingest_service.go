// Package service 业务编排层：只依赖领域接口，不依赖任何 provider SDK 具体类型（基线 §2.2，硬约束 #17）。
package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/ingest"
	"deepwiki/internal/model"
	"deepwiki/internal/queue"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// IngestService 摄取与刷新编排。
type IngestService struct {
	tm        task.TaskManager
	repos     store.RepoStore
	cloner    ingest.Cloner
	publisher queue.Publisher // RabbitMQ 背压预检（QueueDepth），避免白做 LsRemote
	cfg       *config.Manager
	logger    *zap.Logger
}

func NewIngestService(tm task.TaskManager, repos store.RepoStore, cloner ingest.Cloner, publisher queue.Publisher, cfg *config.Manager, logger *zap.Logger) *IngestService {
	return &IngestService{tm: tm, repos: repos, cloner: cloner, publisher: publisher, cfg: cfg, logger: logger}
}

// SubmitIngest POST /api/v1/ingest 的业务实现（§6.1）。
func (s *IngestService) SubmitIngest(ctx context.Context, req dto.IngestRequest) (*model.Task, *model.Repo, error) {
	// TODO: 实现摄取提交，要求：
	// ① s.publisher.QueueDepth 背压预检：≥ x-max-length（worker.queue_size，默认 100）→ 直接返回
	//    model.ErrQueueFull（42902），避免白做 LsRemote（总纲 §4.3 背压契约）；
	// ② cloner.LsRemote 取远端 HEAD commit（git ls-remote，失败放行并记 WARN，§6.1）；
	// ③ 与 repos.GetByURLBranch 比对：commit 未变 → 40901（details 附 existing_repo_id）；
	//    commit 已变 → 40901（details.issue=use_refresh）；
	// ④ 生成 repo_id（"repo_"+ULID）与 task_id（"tsk_"+ULID），Repo 预创建 state=ingesting；
	// ⑤ Task{Type:ingest, State:pending, RequestPayload: 原始请求快照} 经 tm.Submit 提交
	//    （内部：Postgres 落 pending → RabbitMQ 瘦消息 confirm 投递；confirm 失败 → 50302）；
	// ⑥ options 缺省值取 ingest.* 配置（请求 options 覆盖配置，§6.1 校验规则表）。
	panic("TODO: IngestService.SubmitIngest not implemented")
}

// SubmitRefresh POST /api/v1/repos/{repo_id}/refresh 的业务实现（§4.7、§6.7）。
func (s *IngestService) SubmitRefresh(ctx context.Context, repoID string) (*model.Task, error) {
	// TODO: 实现刷新提交，要求：
	// ① repoID 先过 ULID 正则（硬约束 #11）；② 仓库不存在 → 40402；非 ready 或有进行中任务冲突 → 40902；
	// ③ 同仓 refresh 互斥经 Redis 分布式锁 lock:refresh:<repo_id>（SET NX PX 300000，总纲 R13；
	//    多 worker 节点下 v1 原方案的进程内去重机制已失效，必须分布式互斥；持锁失败 → 40902）；
	// ④ 构造 Task{Type:refresh} 经 tm.Submit 提交（Pipeline：fetching→diffing→chunking→embedding→persisting；
	//    git CLI fetch --depth 1 + reset --hard FETCH_HEAD + clean -fdx，禁止 git pull，硬约束 #5）。
	panic("TODO: IngestService.SubmitRefresh not implemented")
}
