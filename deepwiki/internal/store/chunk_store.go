package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// ChunkStore 分块仓储（基线 §7，冻结签名）。
type ChunkStore interface {
	InsertBatch(ctx context.Context, chunks []model.Chunk) error
	GetByID(ctx context.Context, chunkID string) (*model.Chunk, error)
	GetByIDs(ctx context.Context, chunkIDs []string) ([]*model.Chunk, error)
	DeleteByRepo(ctx context.Context, repoID string) error
	// DeleteByPaths refresh 增量删除（modified ∪ deleted 文件对应的 chunks）。
	DeleteByPaths(ctx context.Context, repoID string, paths []string) error
	// FileHashes 按 path 聚合 file_hash，refresh diffing 阶段用（基线 §4.7）。
	FileHashes(ctx context.Context, repoID string) (map[string]string, error)
	Count(ctx context.Context, repoID string) (int64, error)
	// CountByRepo 按仓库统计 chunk 数（启动一致性校验用，总纲 §4.2）。
	CountByRepo(ctx context.Context, repoID string) (int64, error)
}

type pgChunkStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewChunkStore(db *DB, logger *zap.Logger) ChunkStore {
	return &pgChunkStore{pool: db.Pool(), logger: logger}
}

var _ ChunkStore = (*pgChunkStore)(nil)

func (s *pgChunkStore) InsertBatch(ctx context.Context, chunks []model.Chunk) error {
	// TODO: 单事务批量 INSERT（pgx.Batch 或 CopyFrom），要求：
	// ① embedding 列绑定 pgvector.NewVector(c.Vector)；Vector 为 nil 时列写 NULL
	//    （解析切分阶段先插文本行，embedding 阶段再经 VectorStore.Upsert 回补向量）；
	// ② 维度不符会被 vector(1536) 列类型直接拒绝（硬约束 #14 第二道防线）；
	// ③ 全部参数化 $n 占位（硬约束 #11）；批次过大时按 500 条分批；时间 UTC（#13）。
	panic("TODO: pgChunkStore.InsertBatch not implemented")
}

func (s *pgChunkStore) GetByID(ctx context.Context, chunkID string) (*model.Chunk, error) {
	// TODO: 主键查询；pgx.ErrNoRows 透传由上层映射（references 校验 chunk_id 存在，硬约束 #15）。
	panic("TODO: pgChunkStore.GetByID not implemented")
}

func (s *pgChunkStore) GetByIDs(ctx context.Context, chunkIDs []string) ([]*model.Chunk, error) {
	// TODO: WHERE chunk_id = ANY($1)（[]string 直接绑定，禁止循环拼接 IN 列表，硬约束 #11）；
	// 检索回填用：OpenSearch 命中 _id 后批量取 Chunk。
	panic("TODO: pgChunkStore.GetByIDs not implemented")
}

func (s *pgChunkStore) DeleteByRepo(ctx context.Context, repoID string) error {
	// TODO: DELETE FROM chunks WHERE repo_id=$1。
	panic("TODO: pgChunkStore.DeleteByRepo not implemented")
}

func (s *pgChunkStore) DeleteByPaths(ctx context.Context, repoID string, paths []string) error {
	// TODO: DELETE FROM chunks WHERE repo_id=$1 AND path = ANY($2)；refresh persisting 事务内调用（基线 §4.7）。
	panic("TODO: pgChunkStore.DeleteByPaths not implemented")
}

func (s *pgChunkStore) FileHashes(ctx context.Context, repoID string) (map[string]string, error) {
	// TODO: SELECT path, file_hash FROM chunks WHERE repo_id=$1 GROUP BY path（同文件多 chunk 取任一）；
	// 返回 map[path]file_hash 供 diffing 比对（基线 §4.7）。
	panic("TODO: pgChunkStore.FileHashes not implemented")
}

func (s *pgChunkStore) Count(ctx context.Context, repoID string) (int64, error) {
	// TODO: SELECT COUNT(*) FROM chunks WHERE repo_id=$1；repo 详情 chunk_count 与启动时
	// OpenSearch 索引文档数对账用（count(index) == chunks 表行数，不一致 WARN 并后台重建，变更总纲 §4.2）。
	panic("TODO: pgChunkStore.Count not implemented")
}

func (s *pgChunkStore) CountByRepo(ctx context.Context, repoID string) (int64, error) {
	// TODO: SELECT COUNT(*) FROM chunks WHERE repo_id=$1（启动一致性校验用，总纲 §4.2）。
	return 0, nil
}
