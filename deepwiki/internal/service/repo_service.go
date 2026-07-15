package service

import (
	"context"

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
	// TODO: 委托 repos.List；page<1 按 1 处理，pageSize 钳制 1~100 默认 20（§5.4）。
	panic("TODO: RepoService.ListRepos not implemented")
}

// GetRepo GET /api/v1/repos/{repo_id}（§6.7：详情 + latest_task + wiki_available + chunk_count）。
func (s *RepoService) GetRepo(ctx context.Context, repoID string) (*model.Repo, error) {
	// TODO: repos.Get；未命中 → 40402。（latest_task/wiki_available/chunk_count 由 handler 装配或本方法扩展结构，下一轮定）
	panic("TODO: RepoService.GetRepo not implemented")
}

// DeleteRepo DELETE /api/v1/repos/{repo_id}（§12.3 级联矩阵与顺序约定，总纲 §4.1 不变）。
func (s *RepoService) DeleteRepo(ctx context.Context, repoID string) error {
	// TODO: ① repoID ULID 正则校验（硬约束 #11）；
	// ② repos.Delete（DB 事务级联：chunks/wiki_pages CASCADE、tasks.repo_id 置 NULL）；
	// ③ 事务提交后 s.searchCli.DeleteIndex("deepwiki-chunks-"+strings.ToLower(repoID))
	//    （OpenSearch 索引，替代 v1 原方案的 bleve 索引目录）+ 删本地仓库目录；
	//    外部资源失败只记 ERROR 并后台重试清理，不回滚 DB（§12.3，总纲 §4.1）。
	panic("TODO: RepoService.DeleteRepo not implemented")
}
