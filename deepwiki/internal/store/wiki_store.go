package store

import (
	"context"
	"encoding/json"
	"time"

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
	// TODO: 单事务覆盖写（基线 §12.3 wiki 重建）：
	// ① DELETE FROM wiki_pages WHERE repo_id=$1；② 插入 1 行 kind='toc'（toc_json = json.Marshal(w.TOC)，JSONB 列）
	// + N 行 kind='page'；③ 时间写 time.Now().UTC()，timestamptz 列，API 输出 UTC+RFC3339（硬约束 #13）；
	// ④ 全部参数化 $n 占位（硬约束 #11）。
	_ = json.Marshal // 提示：toc_json 用 encoding/json 序列化
	panic("TODO: pgWikiStore.Save not implemented")
}

func (s *pgWikiStore) Get(ctx context.Context, repoID string) (*Wiki, error) {
	// TODO: 读出 toc 行（解析 toc_json）与全部 page 行（按 sort_order 升序）；
	// 无任何行返回 model.ErrWikiNotFound（→ 40403，基线 §6.7）。
	_ = model.ErrWikiNotFound
	panic("TODO: pgWikiStore.Get not implemented")
}

func (s *pgWikiStore) DeleteByRepo(ctx context.Context, repoID string) error {
	// TODO: DELETE FROM wiki_pages WHERE repo_id=$1。
	panic("TODO: pgWikiStore.DeleteByRepo not implemented")
}
