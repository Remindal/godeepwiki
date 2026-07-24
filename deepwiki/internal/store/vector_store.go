package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// VectorStore 向量存储抽象（基线 §7，冻结签名）。
type VectorStore interface {
	Upsert(ctx context.Context, chunks []model.Chunk) error
	Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error)
	DeleteByRepo(ctx context.Context, repoID string) error
}

// pgVectorStore pgvector 实现：embedding vector(1024) 列 + HNSW 索引（变更总纲 §4.1）。
type pgVectorStore struct {
	pool     *pgxpool.Pool
	efSearch int // storage.vector.ef_search，默认 64（可热更新）
	logger   *zap.Logger
}

func NewVectorStore(db *DB, efSearch int, logger *zap.Logger) VectorStore {
	if efSearch <= 0 {
		efSearch = 64
	}
	return &pgVectorStore{pool: db.Pool(), efSearch: efSearch, logger: logger}
}

var _ VectorStore = (*pgVectorStore)(nil)

const vectorBatchSize = 500

func (s *pgVectorStore) Upsert(ctx context.Context, chunks []model.Chunk) error {
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
	for start := 0; start < len(chunks); start += vectorBatchSize {
		end := start + vectorBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]
		values := make([]interface{}, 0, len(batch)*4)
		var placeholders []string
		idx := 1
		for _, c := range batch {
			if len(c.Vector) == 0 {
				continue
			}
			placeholders = append(placeholders, fmt.Sprintf("($%d,$%d,$%d,$%d)", idx, idx+1, idx+2, idx+3))
			values = append(values, c.ChunkID, c.RepoID, c.EmbeddingModel, pgvector.NewVector(c.Vector))
			idx += 4
		}
		if len(placeholders) == 0 {
			continue
		}
		sql := "INSERT INTO chunks (chunk_id, repo_id, embedding_model, embedding) VALUES " +
			joinStrings(placeholders, ",") +
			" ON CONFLICT (chunk_id) DO UPDATE SET embedding = EXCLUDED.embedding, embedding_model = EXCLUDED.embedding_model"
		_, err := s.pool.Exec(ctx, sql, values...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *pgVectorStore) Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error) {
	if err := validateID(repoID); err != nil {
		return nil, err
	}
	if len(vector) == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}
	// 事务内先设置 ef_search，再使用 <=> 算子检索；ef_search 来自内部整型配置，使用 strconv 安全拼接。
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, model.ErrVectorStoreUnavailable
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %s", strconv.Itoa(s.efSearch))); err != nil {
		return nil, model.ErrVectorStoreUnavailable
	}
	rows, err := tx.Query(ctx, `
		SELECT chunk_id, path, start_line, end_line, language, content,
		       1 - (embedding <=> $1) AS score
		FROM chunks
		WHERE repo_id = $2
		ORDER BY embedding <=> $1
		LIMIT $3
	`, pgvector.NewVector(vector), repoID, topK)
	if err != nil {
		return nil, model.ErrVectorStoreUnavailable
	}
	defer rows.Close()
	var hits []model.ChunkHit
	for rows.Next() {
		var h model.ChunkHit
		var score float64
		if err := rows.Scan(&h.Chunk.ChunkID, &h.Chunk.Path, &h.Chunk.StartLine, &h.Chunk.EndLine, &h.Chunk.Language, &h.Chunk.Content, &score); err != nil {
			return nil, model.ErrVectorStoreUnavailable
		}
		h.Score = score
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, model.ErrVectorStoreUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.ErrVectorStoreUnavailable
	}
	return hits, nil
}

func (s *pgVectorStore) DeleteByRepo(ctx context.Context, repoID string) error {
	if err := validateID(repoID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id = $1`, repoID)
	return err
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += sep + ss[i]
	}
	return out
}
