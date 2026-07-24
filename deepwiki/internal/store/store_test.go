package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

func makeFloatSlice(n int, val float32) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = val
	}
	return s
}

// oneHot 生成 n 维 one-hot 向量（idx 位置为 val），余弦距离可区分方向。
func oneHot(n, idx int, val float32) []float32 {
	s := make([]float32, n)
	s[idx] = val
	return s
}

func hashKey(key, salt string) string {
	h := sha256.Sum256([]byte(salt + key))
	return hex.EncodeToString(h[:])
}

func TestStore(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://deepwiki:deepwiki@localhost:5432/deepwiki")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// ===== 1. 向量插入 + 余弦近邻检索 =====

	_, _ = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id='repo_test'`)
	_, err = pool.Exec(ctx,
		`INSERT INTO repos (repo_id,repo_url,branch,created_at,updated_at)
		 VALUES ('repo_test','https://github.com/test','main',now(),now())`,
	)
	if err != nil {
		t.Fatal("insert repo:", err)
	}

	_, err = pool.Exec(ctx, `DELETE FROM chunks WHERE repo_id='repo_test'`)
	if err != nil {
		t.Fatal("cleanup:", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO chunks (chunk_id, repo_id, path, start_line, end_line, language, content, embedding) 
		 VALUES ('chk_01J2X9K7QZ0ABCDEFGHJKMNP','repo_test','a.go',1,10,'go','func main(){}',$1),
		        ('chk_01J2X9K7QZ0ABCDEFGHJKMNQ','repo_test','b.go',1,10,'go','func add(){}',$2),
		        ('chk_01J2X9K7QZ0ABCDEFGHJKMNR','repo_test','c.go',1,10,'go','func mul(){}',$3)
		 ON CONFLICT DO NOTHING`,
		pgvector.NewVector(oneHot(1024, 0, 1.0)),
		pgvector.NewVector(oneHot(1024, 1, 1.0)),
		pgvector.NewVector(oneHot(1024, 2, 1.0)),
	)
	if err != nil {
		t.Fatal("insert chunks:", err)
	}

	var chunkID string
	var score float64
	err = pool.QueryRow(ctx,
		`SELECT chunk_id, 1-(embedding<=>$1) 
		 FROM chunks WHERE repo_id='repo_test' ORDER BY embedding<=>$1 LIMIT 1`,
		pgvector.NewVector(oneHot(1024, 0, 1.0)),
	).Scan(&chunkID, &score)
	if err != nil || chunkID != "chk_01J2X9K7QZ0ABCDEFGHJKMNP" {
		t.Fatalf("vector search wrong: got %s score=%.3f err=%v", chunkID, score, err)
	}
	fmt.Printf("✅ vector search: %s score=%.3f\n", chunkID, score)

	// ===== 2. 维度不符必须被拒绝 =====
	_, err = pool.Exec(ctx,
		`INSERT INTO chunks (chunk_id,repo_id,path,start_line,end_line,language,content,embedding)
		 VALUES ('chk_bad','repo_test','d.go',1,1,'go','x',$1)`,
		pgvector.NewVector(makeFloatSlice(768, 1.0)),
	)
	if err == nil {
		t.Fatal("expected dimension reject, got nil")
	}
	fmt.Printf("✅ dimension reject: %v\n", err)

	// ===== 3. 级联删除 =====
	_, _ = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id='repo_cascade'`)
	_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE task_id='tsk_cascade'`)

	_, err = pool.Exec(ctx,
		`INSERT INTO repos (repo_id,repo_url,branch,created_at,updated_at) 
		 VALUES ('repo_cascade','https://github.com/test','main',now(),now())`,
	)
	_, _ = pool.Exec(ctx,
		`INSERT INTO chunks (chunk_id,repo_id,path,start_line,end_line,language,content,embedding)
		 VALUES ('chk_cascade','repo_cascade','x.go',1,1,'go','x',$1)`,
		pgvector.NewVector(makeFloatSlice(1024, 1.0)),
	)
	_, _ = pool.Exec(ctx,
		`INSERT INTO wiki_pages (repo_id,slug,kind,title,content_md,created_at,updated_at)
		 VALUES ('repo_cascade','overview','page','Overview','x',now(),now())`,
	)
	_, _ = pool.Exec(ctx,
		`INSERT INTO tasks (task_id,type,repo_id,state,created_at)
		 VALUES ('tsk_cascade','ingest','repo_cascade','completed',now())`,
	)

	_, err = pool.Exec(ctx, `DELETE FROM repos WHERE repo_id='repo_cascade'`)
	if err != nil {
		t.Fatal("delete cascade:", err)
	}

	var cnt int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM chunks WHERE repo_id='repo_cascade'`).Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("chunks not cascaded, count=%d", cnt)
	}
	fmt.Println("✅ chunks cascaded")

	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM wiki_pages WHERE repo_id='repo_cascade'`).Scan(&cnt)
	if cnt != 0 {
		t.Fatal("wiki_pages not cascaded")
	}
	fmt.Println("✅ wiki_pages cascaded")

	var repoID *string
	_ = pool.QueryRow(ctx, `SELECT repo_id FROM tasks WHERE task_id='tsk_cascade'`).Scan(&repoID)
	if repoID != nil {
		t.Fatalf("tasks.repo_id not NULL: %v", *repoID)
	}
	fmt.Println("✅ tasks.repo_id set NULL")

	// ===== 4. API key 直接 SQL 验证 =====
	salt := "salttest"
	keyHash := hashKey("testkey123", salt)
	_, err = pool.Exec(ctx,
		`INSERT INTO api_keys (key_id, name, key_hash, salt, is_admin, created_at)
		 VALUES ('key_01J2X9K7QZ0ABCDEFGHJKMNP','test',$1,$2,false,now())
		 ON CONFLICT DO NOTHING`,
		keyHash, salt,
	)
	if err != nil {
		t.Fatal("insert key:", err)
	}
	var foundHash string
	err = pool.QueryRow(ctx, `SELECT key_hash FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL`, keyHash).Scan(&foundHash)
	if err != nil || foundHash != keyHash {
		t.Fatal("key lookup failed")
	}
	fmt.Println("✅ api_key insert + lookup")
}
