package task

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://deepwiki:deepwiki@localhost:5432/deepwiki?sslmode=disable")
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}
	return pool
}

func TestStore_CreateGetUpdateList(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	store := NewTaskStore(pool, zap.NewNop())
	taskID := "tsk_01J2X9K7QZ0ABCDEFGHJKMNP"
	repoID := "repo_01J2X9K7QZ0ABCDEFGHJKMNP"
	_, _ = pool.Exec(ctx, "DELETE FROM tasks WHERE task_id = $1", taskID)
	_, _ = pool.Exec(ctx, "DELETE FROM repos WHERE repo_id = $1", repoID)
	_, _ = pool.Exec(ctx, "DELETE FROM repos WHERE repo_id = $1", repoID)
	_, _ = pool.Exec(ctx, "DELETE FROM chunks WHERE repo_id = $1", repoID)
	_, err := pool.Exec(ctx, "INSERT INTO repos (repo_id, repo_url, branch, created_at, updated_at) VALUES ($1, 'https://github.com/deepwiki-test-store', 'main', now(), now())", repoID)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	task := &model.Task{
		TaskID:    taskID,
		Type:      model.TaskTypeIngest,
		RepoID:    repoID,
		State:     model.TaskStatePending,
		Progress:  model.Progress{Current: 0, Total: 0, Percent: 0},
		Stats:     model.Stats{Files: 0, Chunks: 0, Tokens: 0},
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TaskID != taskID || got.State != model.TaskStatePending {
		t.Fatalf("got %+v", got)
	}

	// pending -> cloning 合法转移。
	cloning := model.TaskStateCloning
	if err := store.UpdateState(ctx, taskID, model.TaskPatch{State: &cloning}); err != nil {
		t.Fatalf("update cloning: %v", err)
	}
	got, _ = store.Get(ctx, taskID)
	if got.State != model.TaskStateCloning {
		t.Fatalf("want cloning got %s", got.State)
	}

	// pending -> failed 非法。
	pending := model.TaskStatePending
	if err := store.UpdateState(ctx, taskID, model.TaskPatch{State: &pending}); !isInvalidState(err) {
		t.Fatalf("expected invalid state error, got %v", err)
	}

	// List 过滤。
	ingestType := model.TaskTypeIngest
	tasks, total, err := store.List(ctx, model.TaskFilter{Type: &ingestType, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 1 || len(tasks) == 0 {
		t.Fatalf("list empty: total=%d", total)
	}

	// Cancel flag。
	if err := store.SetCancelFlag(ctx, taskID); err != nil {
		t.Fatalf("set cancel flag: %v", err)
	}
}

func TestStore_FindInterruptedAndReset(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	store := NewTaskStore(pool, zap.NewNop())
	taskID := "tsk_01J2X9K7QZ0ABCDEFGHJKMNQ"
	_, _ = pool.Exec(ctx, "DELETE FROM tasks WHERE task_id = $1", taskID)

	now := time.Now().UTC()
	task := &model.Task{
		TaskID:    taskID,
		Type:      model.TaskTypeWiki,
		State:     model.TaskStateCloning,
		Progress:  model.Progress{},
		Stats:     model.Stats{},
		CreatedAt: now,
	}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 模拟心跳过期：updated_at 设为 10 分钟前。
	_, _ = pool.Exec(ctx, "UPDATE tasks SET updated_at = $2 WHERE task_id = $1", taskID, now.Add(-10*time.Minute))

	reset, err := store.ResetStaleRunning(ctx, time.Now().UTC().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("reset stale: %v", err)
	}
	if reset != 1 {
		t.Fatalf("want reset 1 got %d", reset)
	}

	interrupted, err := store.FindInterrupted(ctx)
	if err != nil {
		t.Fatalf("find interrupted: %v", err)
	}
	found := false
	for _, it := range interrupted {
		if it.TaskID == taskID {
			found = true
			if it.State != model.TaskStatePending {
				t.Fatalf("want pending after reset got %s", it.State)
			}
		}
	}
	if !found {
		t.Fatal("interrupted task not found")
	}

	// ClaimPending。
	ok, err := store.ClaimPending(ctx, taskID, model.TaskStateOutlining)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !ok {
		t.Fatal("expected claim success")
	}

	// ResetToPending。
	if err := store.ResetToPending(ctx, taskID); err != nil {
		t.Fatalf("reset to pending: %v", err)
	}

	// FailTask。
	if err := store.FailTask(ctx, taskID, &model.TaskError{Code: 50003, Message: "retry exhausted"}); err != nil {
		t.Fatalf("fail task: %v", err)
	}
	got, _ := store.Get(ctx, taskID)
	if got.State != model.TaskStateFailed || got.Err == nil || got.Err.Code != 50003 {
		t.Fatalf("expected failed got %+v", got)
	}
}

func isInvalidState(err error) bool {
	if err == nil {
		return false
	}
	return err == model.ErrInvalidTaskState
}
