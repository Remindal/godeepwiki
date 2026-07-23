package task

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/lock"
	"deepwiki/internal/model"
	"deepwiki/internal/queue"
)

const testRabbitURL2 = "amqp://deepwiki:deepwiki@localhost:5672/"

type fakeBus struct{}

func (f *fakeBus) Publish(ctx context.Context, ev model.Event) error { return nil }
func (f *fakeBus) Subscribe(filter model.EventFilter) (<-chan model.Event, func()) {
	return make(chan model.Event), func() {}
}

var _ eventbus.EventBus = (*fakeBus)(nil)

type testExecutor struct {
	t          *testing.T
	failOnce   bool
	called     int
	delay      time.Duration
	cancelTest bool
}

func (e *testExecutor) Type() model.TaskType { return model.TaskTypeIngest }
func (e *testExecutor) Execute(ctx context.Context, t *model.Task) error {
	e.t.Logf("executor called for %s", t.TaskID)
	e.called++
	if e.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(e.delay):
		}
	}
	if e.cancelTest {
		return ctx.Err()
	}
	// 按合法状态机从 cloning 一路走到 completed（模拟 pipeline）。
	stages := []model.TaskState{
		model.TaskStateParsing,
		model.TaskStateChunking,
		model.TaskStateEmbedding,
		model.TaskStatePersisting,
		model.TaskStateCompleted,
	}
	store := storeFromTest(e.t)
	for _, s := range stages {
		state := s
		patch := model.TaskPatch{State: &state}
		if s == model.TaskStateCompleted {
			now := time.Now().UTC()
			patch.FinishedAt = &now
			patch.Progress = &model.Progress{Current: 1, Total: 1, Percent: 100}
			patch.Stats = &model.Stats{Files: 1}
		}
		if err := store.UpdateState(ctx, t.TaskID, patch); err != nil {
			return err
		}
	}
	return nil
}

var sharedStore *postgresTaskStore

func storeFromTest(t *testing.T) *postgresTaskStore {
	if sharedStore != nil {
		return sharedStore
	}
	pool, err := pgxpool.New(context.Background(), "postgres://deepwiki:deepwiki@localhost:5432/deepwiki?sslmode=disable")
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	sharedStore = NewTaskStore(pool, zap.NewNop())
	return sharedStore
}

func setupManager(t *testing.T, cfg config.WorkerConfig) (*Manager, *queue.Conn, *queue.Consumer, *redis.Client) {
	t.Helper()
	ctx := context.Background()

	pool := testPool(t)
	store := NewTaskStore(pool, zap.NewNop())

	conn, err := queue.Dial(ctx, testRabbitURL2, cfg.QueueSize, zap.NewNop())
	if err != nil {
		t.Skipf("rabbitmq not available: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.DeclareTopology(ctx); err != nil {
		t.Fatalf("declare topology: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}

	publisher := queue.NewPublisher(conn, zap.NewNop())
	consumer := queue.NewConsumer(conn, cfg.PoolSize, zap.NewNop())

	m := NewManager(store, &fakeBus{}, publisher, cfg, zap.NewNop()).
		WithLocker(lock.New(rdb, zap.NewNop())).
		WithMaxRetries(3)
	return m, conn, consumer, rdb
}

func TestManager_SubmitAndConsume(t *testing.T) {
	ctx := context.Background()
	cfg := config.WorkerConfig{PoolSize: 1, QueueSize: 100}
	m, conn, consumer, _ := setupManager(t, cfg)

	ex := &testExecutor{t: t}
	m.RegisterExecutor(ex)

	// 清空队列与历史任务。
	ch, _ := conn.Channel()
	_, _ = ch.QueuePurge(queue.QueueJobs, false)
	_, _ = ch.QueuePurge(queue.QueueDLQ, false)
	_ = ch.Close()

	taskID := "tsk_01J2X9K7QZ0ABCDEFGHJKMNR"
	_, _ = m.store.(*postgresTaskStore).pool.Exec(ctx, "DELETE FROM tasks WHERE task_id = $1", taskID)

	poolCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.Start(poolCtx, consumer)

	task := &model.Task{
		TaskID: taskID,
		Type:   model.TaskTypeIngest,
		RepoID: "",
		State:  model.TaskStatePending,
	}
	if err := m.Submit(ctx, task); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// 等待 worker 消费完成。
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got, err := m.Get(ctx, taskID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.State == model.TaskStateCompleted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	got, err := m.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if got.State != model.TaskStateCompleted {
		t.Fatalf("expected completed got %s", got.State)
	}
	if ex.called == 0 {
		t.Fatal("executor not called")
	}

	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdown()
	m.Stop(shutdown)
}

func TestManager_RefreshLockConflict(t *testing.T) {
	ctx := context.Background()
	cfg := config.WorkerConfig{PoolSize: 1, QueueSize: 100}
	m, _, _, rdb := setupManager(t, cfg)

	repoID := "repo_01J2X9K7QZ0ABCDEFGHJKMNS"
	_ = rdb.Del(ctx, "lock:refresh:"+repoID).Err()
	_, _ = m.store.(*postgresTaskStore).pool.Exec(ctx, "DELETE FROM tasks WHERE task_id IN ('tsk_refresh_1','tsk_refresh_2')")
	_, _ = m.store.(*postgresTaskStore).pool.Exec(ctx, "DELETE FROM repos WHERE repo_id = $1", repoID)
	_, _ = m.store.(*postgresTaskStore).pool.Exec(ctx, "DELETE FROM chunks WHERE repo_id = $1", repoID)
	if _, err := m.store.(*postgresTaskStore).pool.Exec(ctx, "INSERT INTO repos (repo_id, repo_url, branch, created_at, updated_at) VALUES ($1, 'https://github.com/deepwiki-refresh-test', 'main', now(), now())", repoID); err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	task1 := &model.Task{TaskID: "tsk_refresh_1", Type: model.TaskTypeRefresh, RepoID: repoID}
	if err := m.Submit(ctx, task1); err != nil {
		t.Fatalf("submit first refresh: %v", err)
	}

	task2 := &model.Task{TaskID: "tsk_refresh_2", Type: model.TaskTypeRefresh, RepoID: repoID}
	err := m.Submit(ctx, task2)
	if !errors.Is(err, model.ErrRepoAlreadyExists) {
		t.Fatalf("expected lock conflict, got %v", err)
	}

	// 清理锁。
	_ = m.locker.ReleaseRefresh(ctx, repoID, m.refreshLocks[task1.TaskID])
	m.mu.Lock()
	delete(m.refreshLocks, task1.TaskID)
	m.mu.Unlock()
}

func init() {
	_ = amqp.Persistent
}
