package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// WikiTOCItem 目录项（slug 为仓内标识，非全局 ID，基线 §5.6）。
type WikiTOCItem struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	ParentSlug string `json:"parent_slug"`
	SortOrder  int    `json:"sort_order"`
}

// WikiPage 页面。
type WikiPage struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	ContentMD string    `json:"content_md"`
	SortOrder int       `json:"sort_order"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Wiki 一仓的完整 Wiki（TOC + 页面）。
type Wiki struct {
	RepoID      string        `json:"repo_id"`
	TOC         []WikiTOCItem `json:"toc"`
	Pages       []WikiPage    `json:"pages"`
	TaskID      string        `json:"task_id"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// WikiStore Wiki 仓储（基线 §7，冻结签名）。
type WikiStore interface {
	// Save 事务内整体覆盖写（先删该 repo 旧 wiki_pages 再插入 toc 行与 page 行）。
	Save(ctx context.Context, w *Wiki) error
	Get(ctx context.Context, repoID string) (*Wiki, error)
	DeleteByRepo(ctx context.Context, repoID string) error
}

type pgWikiStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewWikiStore(db *DB, logger *zap.Logger) WikiStore {
	return &pgWikiStore{pool: db.Pool(), logger: logger}
}

var _ WikiStore = (*pgWikiStore)(nil)

func (s *pgWikiStore) Save(ctx context.Context, w *Wiki) error {
	if err := validateID(w.RepoID); err != nil {
		return err
	}
	if w.TaskID != "" {
		if err := validateID(w.TaskID); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	tocJSON, _ := json.Marshal(w.TOC)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM wiki_pages WHERE repo_id = $1`, w.RepoID); err != nil {
		return err
	}

	// 写入 TOC 汇总行
	if _, err := tx.Exec(ctx, `
		INSERT INTO wiki_pages (repo_id, slug, kind, title, parent_slug, sort_order, content_md, toc_json, task_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, w.RepoID, "_toc", "toc", "", "", 0, "", tocJSON, w.TaskID, now, now); err != nil {
		return err
	}

	// 写入页面行
	for _, p := range w.Pages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wiki_pages (repo_id, slug, kind, title, parent_slug, sort_order, content_md, toc_json, task_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, w.RepoID, p.Slug, "page", p.Title, "", p.SortOrder, p.ContentMD, nil, w.TaskID, now, now); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *pgWikiStore) Get(ctx context.Context, repoID string) (*Wiki, error) {
	if err := validateID(repoID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT repo_id, slug, kind, title, parent_slug, sort_order, content_md, toc_json, task_id, created_at, updated_at
		FROM wiki_pages
		WHERE repo_id = $1
		ORDER BY kind DESC, sort_order ASC, slug ASC
	`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	wiki := &Wiki{RepoID: repoID}
	var found bool
	for rows.Next() {
		found = true
		var slug, kind, title, parentSlug string
		var sortOrder int
		var contentMD string
		var tocJSON []byte
		var taskID string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&wiki.RepoID, &slug, &kind, &title, &parentSlug, &sortOrder, &contentMD, &tocJSON, &taskID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		wiki.TaskID = taskID
		switch kind {
		case "toc":
			_ = json.Unmarshal(tocJSON, &wiki.TOC)
			wiki.GeneratedAt = updatedAt
		case "page":
			wiki.Pages = append(wiki.Pages, WikiPage{
				Slug:      slug,
				Title:     title,
				ContentMD: contentMD,
				SortOrder: sortOrder,
				UpdatedAt: updatedAt,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, model.ErrWikiNotFound
	}
	return wiki, nil
}

func (s *pgWikiStore) DeleteByRepo(ctx context.Context, repoID string) error {
	if err := validateID(repoID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM wiki_pages WHERE repo_id = $1`, repoID)
	return err
}
