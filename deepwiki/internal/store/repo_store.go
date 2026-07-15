package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
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
	if err := validateID(r.RepoID); err != nil {
		return err
	}
	now := time.Now().UTC()
	statsJSON, _ := json.Marshal(r.Stats)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO repos (repo_id, repo_url, branch, commit_hash, local_path, state, stats_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, r.RepoID, r.RepoURL, r.Branch, r.CommitHash, r.LocalPath, r.State, statsJSON, now, now)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return model.ErrRepoAlreadyExists
		}
		return err
	}
	return nil
}

func (s *pgRepoStore) Get(ctx context.Context, repoID string) (*model.Repo, error) {
	if err := validateID(repoID); err != nil {
		return nil, err
	}
	row := s.pool.QueryRow(ctx, `
		SELECT repo_id, repo_url, branch, commit_hash, local_path, state, stats_json, created_at, updated_at
		FROM repos WHERE repo_id = $1
	`, repoID)
	var r model.Repo
	var statsJSON []byte
	err := row.Scan(&r.RepoID, &r.RepoURL, &r.Branch, &r.CommitHash, &r.LocalPath, &r.State, &statsJSON, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrRepoNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(statsJSON, &r.Stats)
	return &r, nil
}

func (s *pgRepoStore) GetByURLBranch(ctx context.Context, url, branch string) (*model.Repo, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT repo_id, repo_url, branch, commit_hash, local_path, state, stats_json, created_at, updated_at
		FROM repos WHERE repo_url = $1 AND branch = $2
	`, url, branch)
	var r model.Repo
	var statsJSON []byte
	err := row.Scan(&r.RepoID, &r.RepoURL, &r.Branch, &r.CommitHash, &r.LocalPath, &r.State, &statsJSON, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrRepoNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(statsJSON, &r.Stats)
	return &r, nil
}

func (s *pgRepoStore) Update(ctx context.Context, r *model.Repo) error {
	if err := validateID(r.RepoID); err != nil {
		return err
	}
	now := time.Now().UTC()
	statsJSON, _ := json.Marshal(r.Stats)
	_, err := s.pool.Exec(ctx, `
		UPDATE repos
		SET repo_url = $1, branch = $2, commit_hash = $3, local_path = $4,
		    state = $5, stats_json = $6, updated_at = $7
		WHERE repo_id = $8
	`, r.RepoURL, r.Branch, r.CommitHash, r.LocalPath, r.State, statsJSON, now, r.RepoID)
	return err
}

func (s *pgRepoStore) List(ctx context.Context, page, pageSize int) ([]*model.Repo, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM repos`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT repo_id, repo_url, branch, commit_hash, local_path, state, stats_json, created_at, updated_at
		FROM repos ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var repos []*model.Repo
	for rows.Next() {
		var r model.Repo
		var statsJSON []byte
		if err := rows.Scan(&r.RepoID, &r.RepoURL, &r.Branch, &r.CommitHash, &r.LocalPath, &r.State, &statsJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(statsJSON, &r.Stats)
		repos = append(repos, &r)
	}
	return repos, total, rows.Err()
}

func (s *pgRepoStore) Delete(ctx context.Context, repoID string) error {
	if err := validateID(repoID); err != nil {
		return err
	}
	// 单事务只删除 repos 行；chunks/wiki_pages 由 ON DELETE CASCADE 处理，tasks.repo_id 由 ON DELETE SET NULL 处理。
	_, err := s.pool.Exec(ctx, `DELETE FROM repos WHERE repo_id = $1`, repoID)
	return err
}

func (s *pgRepoStore) ListRepoIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT repo_id FROM repos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
