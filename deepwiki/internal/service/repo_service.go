package service

import (
	"context"
	"errors"
	"os"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// RepoService 仓库资源族（列表/详情/删除）。
type RepoService struct {
	repos     store.RepoStore
	chunks    store.ChunkStore
	vectors   store.VectorStore
	wikis     store.WikiStore
	searchCli *search.Client // OpenSearch 索引生命周期（删除仓库时删索引，总纲 §4.2）
	tm        task.TaskManager
	logger    *zap.Logger
}

func NewRepoService(repos store.RepoStore, chunks store.ChunkStore, vectors store.VectorStore, wikis store.WikiStore, searchCli *search.Client, tm task.TaskManager, logger *zap.Logger) *RepoService {
	return &RepoService{repos: repos, chunks: chunks, vectors: vectors, wikis: wikis, searchCli: searchCli, tm: tm, logger: logger}
}

// ListRepos GET /api/v1/repos（§5.4 分页，created_at DESC）。
func (s *RepoService) ListRepos(ctx context.Context, page, pageSize int) ([]*model.Repo, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repos.List(ctx, page, pageSize)
}

// RepoDetail GET /api/v1/repos/{repo_id} 的响应 data（§6.7）。
type RepoDetail struct {
	*model.Repo
	LatestTask    *model.Task `json:"latest_task,omitempty"`
	WikiAvailable bool        `json:"wiki_available"`
	ChunkCount    int64       `json:"chunk_count"`
}

// GetRepo GET /api/v1/repos/{repo_id}（§6.7：详情 + latest_task + wiki_available + chunk_count）。
func (s *RepoService) GetRepo(ctx context.Context, repoID string) (*RepoDetail, error) {
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

	detail := &RepoDetail{Repo: repo}
	if tasks, _, err := s.tm.List(ctx, model.TaskFilter{RepoID: repoID, Page: 1, PageSize: 1}); err == nil && len(tasks) > 0 {
		detail.LatestTask = tasks[0]
	}
	if _, err := s.wikis.Get(ctx, repoID); err == nil {
		detail.WikiAvailable = true
	}
	if n, err := s.chunks.Count(ctx, repoID); err == nil {
		detail.ChunkCount = n
	}
	return detail, nil
}

// RepoDeleteResult DeleteRepo 的删除统计（§12.3 响应 data）。
type RepoDeleteResult struct {
	Chunks        int64 `json:"chunks"`
	Vectors       int64 `json:"vectors"`
	WikiPages     int64 `json:"wiki_pages"`
	OpenSearchDocs int64 `json:"opensearch_docs"`
	LocalDir      bool  `json:"local_dir"`
}

// DeleteRepo DELETE /api/v1/repos/{repo_id}（§12.3 级联矩阵与顺序约定，总纲 §4.1 不变）。
func (s *RepoService) DeleteRepo(ctx context.Context, repoID string) (*RepoDeleteResult, error) {
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

	res := &RepoDeleteResult{}
	if n, err := s.chunks.Count(ctx, repoID); err == nil {
		res.Chunks = n
		res.Vectors = n
	}
	if w, err := s.wikis.Get(ctx, repoID); err == nil && w != nil {
		res.WikiPages = int64(len(w.Pages))
	}
	if s.searchCli != nil {
		if n, err := s.searchCli.Count(ctx, repoID); err == nil {
			res.OpenSearchDocs = n
		}
	}

	// ② DB 事务级联：chunks/wiki_pages CASCADE、tasks.repo_id 置 NULL。
	if err := s.repos.Delete(ctx, repoID); err != nil {
		return nil, err
	}

	// ③ 事务提交后删 OpenSearch 索引与本地目录；外部资源失败只记 ERROR，不回滚 DB。
	if s.searchCli != nil {
		if err := s.searchCli.DeleteIndex(ctx, repoID); err != nil {
			s.logger.Error("delete opensearch index failed", zap.String("repo_id", repoID), zap.Error(err))
		}
	}
	if repo.LocalPath != "" {
		if err := os.RemoveAll(repo.LocalPath); err != nil {
			s.logger.Error("delete local repo dir failed", zap.String("repo_id", repoID), zap.String("path", repo.LocalPath), zap.Error(err))
		} else {
			res.LocalDir = true
		}
	}
	return res, nil
}
