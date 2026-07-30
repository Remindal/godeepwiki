package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
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

const chunkBatchSize = 500

var chunkColumns = []string{
	"chunk_id", "repo_id", "path", "start_line", "end_line",
	"language", "content", "file_hash", "embedding_model", "embedding", "parent_chunk_id",
}

func (s *pgChunkStore) InsertBatch(ctx context.Context, chunks []model.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	for _, c := range chunks {
		if err := validateID(c.ChunkID); err != nil {
			return err
		}
		if err := validateID(c.RepoID); err != nil {
			return err
		}
	}
	for start := 0; start < len(chunks); start += chunkBatchSize {
		end := start + chunkBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		// 参数化多行 INSERT（CopyFrom 对 pgvector.Vector 编码有误，二进制 COPY 下会报
		// "vector cannot have more than 16000 dimensions"，参数化 INSERT 为已验证路径）。
		sql, args := buildChunkInsert(chunks[start:end])
		if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
			return err
		}
	}
	return nil
}

// buildChunkInsert 构造 INSERT INTO chunks (...) VALUES (...),(...),... 与参数（每行 11 个 $n）。
func buildChunkInsert(chunks []model.Chunk) (string, []interface{}) {
	const cols = 11
	var sb strings.Builder
	sb.WriteString("INSERT INTO chunks (")
	sb.WriteString(strings.Join(chunkColumns, ", "))
	sb.WriteString(") VALUES ")
	args := make([]interface{}, 0, len(chunks)*cols)
	for i, c := range chunks {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(")
		base := i * cols
		for j := 0; j < cols; j++ {
			if j > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, "$%d", base+j+1)
		}
		sb.WriteString(")")
		var emb interface{}
		if len(c.Vector) > 0 {
			emb = pgvector.NewVector(c.Vector)
		}
		var parentID interface{}
		if c.ParentChunkID != "" {
			parentID = c.ParentChunkID
		}
		args = append(args,
			c.ChunkID, c.RepoID, c.Path, c.StartLine, c.EndLine,
			c.Language, c.Content, c.FileHash, c.EmbeddingModel, emb, parentID,
		)
	}
	return sb.String(), args
}

func (s *pgChunkStore) GetByID(ctx context.Context, chunkID string) (*model.Chunk, error) {
	if err := validateID(chunkID); err != nil {
		return nil, err
	}
	row := s.pool.QueryRow(ctx, `
		SELECT chunk_id, repo_id, path, start_line, end_line, language, content, file_hash, embedding_model, embedding, parent_chunk_id
		FROM chunks WHERE chunk_id = $1
	`, chunkID)
	var c model.Chunk
	var vec *pgvector.Vector
	var parentID *string
	err := row.Scan(&c.ChunkID, &c.RepoID, &c.Path, &c.StartLine, &c.EndLine, &c.Language, &c.Content, &c.FileHash, &c.EmbeddingModel, &vec, &parentID)
	if err != nil {
		return nil, err
	}
	if vec != nil {
		c.Vector = vec.Slice()
	}
	if parentID != nil {
		c.ParentChunkID = *parentID
	}
	return &c, nil
}

func (s *pgChunkStore) GetByIDs(ctx context.Context, chunkIDs []string) ([]*model.Chunk, error) {
	for _, id := range chunkIDs {
		if err := validateID(id); err != nil {
			return nil, err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT chunk_id, repo_id, path, start_line, end_line, language, content, file_hash, embedding_model, embedding, parent_chunk_id
		FROM chunks WHERE chunk_id = ANY($1)
	`, chunkIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Chunk
	for rows.Next() {
		var c model.Chunk
		var vec *pgvector.Vector
		var parentID *string
		if err := rows.Scan(&c.ChunkID, &c.RepoID, &c.Path, &c.StartLine, &c.EndLine, &c.Language, &c.Content, &c.FileHash, &c.EmbeddingModel, &vec, &parentID); err != nil {
			return nil, err
		}
		if vec != nil {
			c.Vector = vec.Slice()
		}
		if parentID != nil {
			c.ParentChunkID = *parentID
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (s *pgChunkStore) DeleteByRepo(ctx context.Context, repoID string) error {
	if err := validateID(repoID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id = $1`, repoID)
	return err
}

func (s *pgChunkStore) DeleteByPaths(ctx context.Context, repoID string, paths []string) error {
	if err := validateID(repoID); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id = $1 AND path = ANY($2)`, repoID, paths)
	return err
}

func (s *pgChunkStore) FileHashes(ctx context.Context, repoID string) (map[string]string, error) {
	if err := validateID(repoID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT path, file_hash FROM chunks WHERE repo_id = $1
	`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		if _, ok := m[path]; !ok {
			m[path] = hash
		}
	}
	return m, rows.Err()
}

func (s *pgChunkStore) Count(ctx context.Context, repoID string) (int64, error) {
	return s.CountByRepo(ctx, repoID)
}

func (s *pgChunkStore) CountByRepo(ctx context.Context, repoID string) (int64, error) {
	if err := validateID(repoID); err != nil {
		return 0, err
	}
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chunks WHERE repo_id = $1`, repoID).Scan(&n)
	return n, err
}
