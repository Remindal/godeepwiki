package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// WikiService Wiki 编排（与 ingest 共用同一套任务系统，建议⑩）。
type WikiService struct {
	tm     task.TaskManager
	wikis  store.WikiStore
	logger *zap.Logger
}

func NewWikiService(tm task.TaskManager, wikis store.WikiStore, logger *zap.Logger) *WikiService {
	return &WikiService{tm: tm, wikis: wikis, logger: logger}
}

// Generate POST /api/v1/wiki/generate（§6.7）：返回 202 + task_id；已有 wiki 则覆盖重建。
func (s *WikiService) Generate(ctx context.Context, repoID string) (*model.Task, error) {
	// TODO: ① repoID ULID 正则校验；仓库须 ready，否则 40902；② 构造 Task{Type:wiki, State:pending} 经 tm.Submit 提交
	// （wiki 状态机 pending→outlining→generating→completed，§4.3；L2 配额走 wiki_per_hour=10，总纲 §2.8）；
	// ③ auto_wiki=true 的 ingest 进入 completed 后由 TaskManager 自动调用本方法（§4.3 级联联动）。
	panic("TODO: WikiService.Generate not implemented")
}

// GetWiki GET /api/v1/repos/{repo_id}/wiki（§6.7）：未生成 → 40403。
func (s *WikiService) GetWiki(ctx context.Context, repoID string) (*store.Wiki, error) {
	// TODO: 委托 wikis.Get；model.ErrWikiNotFound → 40403。
	panic("TODO: WikiService.GetWiki not implemented")
}
