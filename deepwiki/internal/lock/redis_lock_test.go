package lock

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}
	return rdb
}

func TestLocker_LockUnlock(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	defer rdb.Close()

	l := New(rdb, zap.NewNop())
	key := "lock:test:" + time.Now().Format("20060102150405.000")

	ok, err := l.Lock(ctx, key, "token-a", time.Second*5)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected lock acquired")
	}

	// 同 key 不同 token 应失败。
	ok2, err := l.Lock(ctx, key, "token-b", time.Second*5)
	if err != nil {
		t.Fatal(err)
	}
	if ok2 {
		t.Fatal("expected lock conflict")
	}

	// 错误 token 不能解锁。
	if err := l.Unlock(ctx, key, "token-b"); err != nil {
		t.Fatal(err)
	}
	still, _ := l.Lock(ctx, key, "token-c", time.Second*5)
	if still {
		t.Fatal("expected still locked")
	}

	// 正确 token 解锁。
	if err := l.Unlock(ctx, key, "token-a"); err != nil {
		t.Fatal(err)
	}
	ok3, err := l.Lock(ctx, key, "token-d", time.Second*5)
	if err != nil {
		t.Fatal(err)
	}
	if !ok3 {
		t.Fatal("expected reacquire after unlock")
	}
	_ = l.Unlock(ctx, key, "token-d")
}

func TestLocker_AcquireRefresh(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	defer rdb.Close()

	l := New(rdb, zap.NewNop())
	repoID := "repo_01J2X9K7QZ0ABCDEFGHJKMNP"
	_ = rdb.Del(ctx, "lock:refresh:"+repoID).Err()
	defer rdb.Del(ctx, "lock:refresh:"+repoID)

	token1, ok1, err := l.AcquireRefresh(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok1 || token1 == "" {
		t.Fatal("expected acquire refresh lock")
	}

	_, ok2, _ := l.AcquireRefresh(ctx, repoID)
	if ok2 {
		t.Fatal("expected second refresh lock rejected")
	}

	if err := l.ReleaseRefresh(ctx, repoID, token1); err != nil {
		t.Fatal(err)
	}

	_, ok3, err := l.AcquireRefresh(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok3 {
		t.Fatal("expected refresh lock reacquired")
	}
	_ = l.ReleaseRefresh(ctx, repoID, token1)
}
