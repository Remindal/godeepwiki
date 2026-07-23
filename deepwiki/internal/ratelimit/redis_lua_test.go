package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}
	return rdb
}

func TestRedisLimiter_SlidingWindow(t *testing.T) {
	ctx := context.Background()
	rdb := testRedisClient(t)
	defer rdb.Close()

	l := NewRedisLimiter(rdb, zap.NewNop())
	defer l.Close()

	key := "rl:test:" + time.Now().Format("20060102150405.000")
	_ = rdb.Del(ctx, key)

	d1, err := l.Allow(ctx, key, time.Minute, 2)
	if err != nil {
		t.Fatalf("allow 1: %v", err)
	}
	if !d1.Allowed || d1.Remaining != 1 {
		t.Fatalf("d1: %+v", d1)
	}
	d2, err := l.Allow(ctx, key, time.Minute, 2)
	if err != nil {
		t.Fatalf("allow 2: %v", err)
	}
	if !d2.Allowed || d2.Remaining != 0 {
		t.Fatalf("d2: %+v", d2)
	}
	d3, err := l.Allow(ctx, key, time.Minute, 2)
	if err != nil {
		t.Fatalf("allow 3: %v", err)
	}
	if d3.Allowed {
		t.Fatalf("d3 should be rejected: %+v", d3)
	}
	if d3.RetryAfter <= 0 {
		t.Fatalf("d3 retry-after not set: %+v", d3)
	}
	if l.Degraded() {
		t.Fatal("should not be degraded")
	}
}

func TestRedisLimiter_DegradedFallback(t *testing.T) {
	ctx := context.Background()
	badRdb := redis.NewClient(&redis.Options{Addr: "localhost:6399"}) // 不存在的 Redis
	defer badRdb.Close()

	l := NewRedisLimiter(badRdb, zap.NewNop())
	defer l.Close()

	key := "rl:test:degraded:" + time.Now().Format("20060102150405.000")
	d, err := l.Allow(ctx, key, time.Minute, 1)
	if err != nil {
		t.Fatalf("fallback should not error: %v", err)
	}
	if !d.Degraded {
		t.Fatalf("expected degraded decision: %+v", d)
	}
	if !l.Degraded() {
		t.Fatal("limiter should be degraded")
	}
	// 第二次仍应被进程内限流拦住。
	d2, _ := l.Allow(ctx, key, time.Minute, 1)
	if d2.Allowed {
		t.Fatalf("fallback should reject second request: %+v", d2)
	}
}
