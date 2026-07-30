package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
)

// TestVerifyIndicesRebuild 模拟索引与 chunks 表行数不一致，验证 verifyIndices 后台重建后计数一致。
func TestVerifyIndicesRebuild(t *testing.T) {
	if conn, err := net.DialTimeout("tcp", "localhost:5432", 2*time.Second); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	} else {
		_ = conn.Close()
	}
	if conn, err := net.DialTimeout("tcp", "localhost:9200", 2*time.Second); err != nil {
		t.Skipf("opensearch unreachable: %v", err)
	} else {
		_ = conn.Close()
	}

	ctx := context.Background()
	db, err := store.Open(ctx, "postgres://deepwiki:deepwiki@localhost:5432/deepwiki", 5, zap.NewNop())
	if err != nil {
		t.Skipf("postgres open: %v", err)
	}
	defer db.Close()
	pool := db.Pool()

	osCli, err := search.NewClient(ctx, config.OpenSearchConfig{
		Addresses:   []string{"http://localhost:9200"},
		IndexPrefix: "deepwiki",
	}, zap.NewNop())
	if err != nil {
		t.Skipf("opensearch client: %v", err)
	}

	repoID := "repo_01J2X9K7QZ0ABCDEFGHJKMNQR9"
	parentIDs := []string{
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQP9",
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQPA",
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQPB",
	}
	chunkIDs := []string{
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQR9",
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQRA",
		"chk_01J2X9K7QZ0ABCDEFGHJKMNQRB",
	}
	_, _ = pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id = $1`, repoID)
	_, _ = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id = $1`, repoID)
	_ = osCli.DeleteIndex(ctx, repoID)
	if _, err := pool.Exec(ctx, `INSERT INTO repos (repo_id, repo_url, branch, created_at, updated_at) VALUES ($1, 'https://github.com/it/verify', 'main', now(), now())`, repoID); err != nil {
		t.Fatalf("insert repo: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM chunks WHERE repo_id = $1`, repoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM repos WHERE repo_id = $1`, repoID)
		_ = osCli.DeleteIndex(context.Background(), repoID)
	}()
	vec := make([]float32, 1024)
	vec[0] = 1.0
	// 父子块结构：父块无向量不进索引，子块带向量（OpenSearch 装块口径 = 子块数）。
	for i, id := range parentIDs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO chunks (chunk_id, repo_id, path, start_line, end_line, language, content, file_hash, embedding_model, embedding)
			VALUES ($1, $2, $3, 1, 5, 'go', $4, 'hash', 'it-model', NULL)
		`, id, repoID, "a.go", "package a"); err != nil {
			t.Fatalf("insert parent chunk %d: %v", i, err)
		}
	}
	for i, id := range chunkIDs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO chunks (chunk_id, repo_id, path, start_line, end_line, language, content, file_hash, embedding_model, embedding, parent_chunk_id)
			VALUES ($1, $2, $3, 1, 5, 'go', $4, 'hash', 'it-model', $5, $6)
		`, id, repoID, "a.go", "package a", pgvector.NewVector(vec), parentIDs[i]); err != nil {
			t.Fatalf("insert chunk %d: %v", i, err)
		}
	}

	// 建立空索引制造不一致（postgres=3 子块，opensearch=0）
	if err := osCli.CreateIndex(ctx, repoID); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}

	verifyIndices(ctx, store.NewRepoStore(db, zap.NewNop()), store.NewChunkStore(db, zap.NewNop()), osCli, pool, zap.NewNop())

	if err := osCli.Refresh(ctx, repoID); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	got, err := osCli.Count(ctx, repoID)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got != int64(len(chunkIDs)) {
		t.Fatalf("after rebuild Count = %d, want %d", got, len(chunkIDs))
	}
}
