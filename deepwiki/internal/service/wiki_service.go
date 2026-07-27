package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// WikiService Wiki 编排（与 ingest 共用同一套任务系统，建议⑩）。
type WikiService struct {
	tm     task.TaskManager
	repos  store.RepoStore
	wikis  store.WikiStore
	logger *zap.Logger
}

func NewWikiService(tm task.TaskManager, repos store.RepoStore, wikis store.WikiStore, logger *zap.Logger) *WikiService {
	return &WikiService{tm: tm, repos: repos, wikis: wikis, logger: logger}
}

// Generate POST /api/v1/wiki/generate（§6.7）：返回 202 + task_id；已有 wiki 则覆盖重建。
func (s *WikiService) Generate(ctx context.Context, repoID string) (*model.Task, error) {
	if !repoIDRegex.MatchString(repoID) {
		return nil, model.NewAPIError(model.CodeInvalidParam, "invalid repo_id format")
	}
	repo, err := s.repos.Get(ctx, repoID)
	if err != nil {
		if errors.Is(err, model.ErrRepoNotFound) {
			return nil, model.ErrRepoNotFound
		}
		return nil, err
	}
	if repo.State != "ready" {
		return nil, model.NewAPIError(model.CodeInvalidTaskState, "repo is not ready")
	}

	// 生成中互斥：该仓库存在非终态任务（wiki/ingest/refresh）时拒绝重复提交，
	// 避免同一仓库的 wiki 任务重复排队（用户期望提示"正在生成"）。
	if tasks, _, err := s.tm.List(ctx, model.TaskFilter{RepoID: repoID, Page: 1, PageSize: 100}); err == nil {
		for _, t := range tasks {
			if !t.State.IsTerminal() {
				return nil, model.NewAPIError(model.CodeInvalidTaskState, "wiki 正在生成中，请等待完成")
			}
		}
	}

	t := &model.Task{
		TaskID:    newID("tsk_"),
		Type:      model.TaskTypeWiki,
		RepoID:    repo.RepoID,
		State:     model.TaskStatePending,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.tm.Submit(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetWiki GET /api/v1/repos/{repo_id}/wiki（§6.7）：未生成 → 40403。
func (s *WikiService) GetWiki(ctx context.Context, repoID string) (*store.Wiki, error) {
	if !repoIDRegex.MatchString(repoID) {
		return nil, model.NewAPIError(model.CodeInvalidParam, "invalid repo_id format")
	}
	w, err := s.wikis.Get(ctx, repoID)
	if err != nil {
		return nil, err
	}
	return w, nil
}
