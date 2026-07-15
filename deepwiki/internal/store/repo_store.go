package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// RepoStore 仓库仓储（基线 §7，冻结签名）。
type RepoStore interface {
	Create(ctx context.Context, r *model.Repo) error
	Get(ctx context.Context, repoID string) (*model.Repo, error)
	GetByURLBranch(ctx context.Context, url, branch string) (*model.Repo, error)
	Update(ctx context.Context, r *model.Repo) error
	List(ctx context.Context, page, pageSize int) (repos []*model.Repo, total int64, err error)
	// ListRepoIDs 返回全部仓库 ID（启动一致性校验用，总纲 §4.2）。
	ListRepoIDs(ctx context.Context) ([]string, error)
	// Delete 事务级联：chunks/wiki_pages 随 ON DELETE CASCADE 删除；tasks.repo_id 置 NULL；
	// 事务提交后再删 OpenSearch 索引与本地目录（基线 §12.3 顺序约定）。
	Delete(ctx context.Context, repoID string) error
}

type pgRepoStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewRepoStore 返回 RepoStore 的 PostgreSQL 实现。
func NewRepoStore(db *DB, logger *zap.Logger) RepoStore {
	return &pgRepoStore{pool: db.Pool(), logger: logger}
}

var _ RepoStore = (*pgRepoStore)(nil)

func (s *pgRepoStore) Create(ctx context.Context, r *model.Repo) error {
	// TODO: INSERT INTO repos (repo_id, repo_url, branch, commit_hash, local_path, state, stats_json, created_at, updated_at)
	// VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)；stats_json 用 json.Marshal(r.Stats) 绑定（JSONB 列）；
	// 全部参数化 $n 占位（硬约束 #11）；时间写 time.Now().UTC()，列类型 timestamptz，API 输出 UTC+RFC3339（#13）；
	// UNIQUE(repo_url,branch) 冲突（pg 错误码 23505）映射 model.ErrRepoAlreadyExists。
	panic("TODO: pgRepoStore.Create not implemented")
}

func (s *pgRepoStore) Get(ctx context.Context, repoID string) (*model.Repo, error) {
	// TODO: 按主键查询；pgx.ErrNoRows 映射 model.ErrRepoNotFound；stats_json 反序列化为 model.RepoStats；
	// repoID 入参必须先过 ULID 正则（硬约束 #11）。
	panic("TODO: pgRepoStore.Get not implemented")
}

func (s *pgRepoStore) GetByURLBranch(ctx context.Context, url, branch string) (*model.Repo, error) {
	// TODO: 同 Get；未命中返回 model.ErrRepoNotFound（ingest 幂等判断用，基线 §6.1）。
	panic("TODO: pgRepoStore.GetByURLBranch not implemented")
}

func (s *pgRepoStore) Update(ctx context.Context, r *model.Repo) error {
	// TODO: 全列更新（state/commit_hash/local_path/stats_json/updated_at）；updated_at 由本方法刷新为当前 UTC。
	panic("TODO: pgRepoStore.Update not implemented")
}

func (s *pgRepoStore) List(ctx context.Context, page, pageSize int) ([]*model.Repo, int64, error) {
	// TODO: created_at DESC 排序 + LIMIT $1 OFFSET $2；返回 total（COUNT(*)）；越界返回空 items 与真实 total（基线 §5.4）。
	panic("TODO: pgRepoStore.List not implemented")
}

func (s *pgRepoStore) Delete(ctx context.Context, repoID string) error {
	// TODO: 级联删除（基线 §12.3 顺序约定，变更总纲 §4.1 级联删除矩阵不变）：
	// ① 单事务内 DELETE repos 行（chunks/wiki_pages 靠 ON DELETE CASCADE，tasks.repo_id 靠 ON DELETE SET NULL）；
	// ② 事务提交后再删 OpenSearch 索引（deepwiki-chunks-<repo_id 全小写>）与本地仓库目录，
	//    外部资源失败只记 ERROR 日志并后台重试清理，不回滚 DB。
	panic("TODO: pgRepoStore.Delete not implemented")
}

func (s *pgRepoStore) ListRepoIDs(ctx context.Context) ([]string, error) {
	// TODO: SELECT repo_id FROM repos（启动一致性校验用，总纲 §4.2）。
	return nil, nil
}
