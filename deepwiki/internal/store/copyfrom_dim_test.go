package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"deepwiki/internal/model"
)

// TestCopyFromVectorDim 验证参数化 INSERT + pgvector 写入 vector(1024)
//（CopyFrom 对 pgvector.Vector 编码有误，已废弃，见 buildChunkInsert 注释）。
func TestCopyFromVectorDim(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://deepwiki:deepwiki@localhost:5432/deepwiki?sslmode=disable")
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer pool.Close()

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.01
	}

	_, _ = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id = 'repo_copytest'`)
	if _, err := pool.Exec(ctx, `INSERT INTO repos (repo_id, repo_url, branch, created_at, updated_at) VALUES ('repo_copytest', 'https://github.com/copytest', 'main', now(), now())`); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id = 'repo_copytest'`)
	_, _ = pool.Exec(ctx, `DELETE FROM chunks WHERE chunk_id = 'chk_01J2X9K7QZ0ABCDEFGHJKMNR'`)

	sql, args := buildChunkInsert([]model.Chunk{{
		ChunkID: "chk_01J2X9K7QZ0ABCDEFGHJKMNR", RepoID: "repo_copytest", Path: "a.go",
		StartLine: 1, EndLine: 10, Language: "go", Content: "content", FileHash: "hash",
		EmbeddingModel: "bge", Vector: vec,
	}})
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var dims int
	if err := pool.QueryRow(ctx, `SELECT vector_dims(embedding) FROM chunks WHERE repo_id='repo_copytest'`).Scan(&dims); err != nil {
		t.Fatal(err)
	}
	if dims != 1024 {
		t.Fatalf("want 1024 got %d", dims)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id='repo_copytest'`)
	_, _ = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id='repo_copytest'`)
}
