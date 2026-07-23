package ratelimit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// slidingWindowLua 滑动窗口限流脚本（总纲 §4.4 权威脚本，逐字一致，禁止改动）：
// ZSET 成员为请求时间戳，窗口外成员先剔除再计数，原子完成「判定+记账+过期」。
const slidingWindowLua = `
-- KEYS[1]=窗口键  ARGV=now_ms, window_ms, limit
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[1]-ARGV[2])
local n = redis.call('ZCARD', KEYS[1])
if n < tonumber(ARGV[3]) then
  redis.call('ZADD', KEYS[1], ARGV[1], ARGV[1]..'-'..math.random(1,1e9))
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return {1, tonumber(ARGV[3])-n-1}
end
return {0, 0}
`

// redisLimiter Redis 滑动窗口限流器（Lua 原子执行；哨兵高可用经 go-redis FailoverClient，总纲 R11）。
type redisLimiter struct {
	rdb      redis.UniversalClient
	script   *redis.Script
	fallback *fallbackLimiter
	degraded atomic.Bool
	logger   *zap.Logger
}

func NewRedisLimiter(rdb redis.UniversalClient, logger *zap.Logger) Limiter {
	return &redisLimiter{
		rdb:      rdb,
		script:   redis.NewScript(slidingWindowLua),
		fallback: newFallbackLimiter(),
		logger:   logger,
	}
}

func (l *redisLimiter) Allow(ctx context.Context, key string, window time.Duration, limit int) (Decision, error) {
	now := time.Now().UTC()
	nowMs := now.UnixMilli()
	res, err := l.script.Run(ctx, l.rdb, []string{key}, nowMs, window.Milliseconds(), limit).Result()
	if err != nil {
		l.degraded.Store(true)
		l.logger.Warn("redis ratelimit failed, fallback to in-process", zap.Error(err))
		return l.fallback.allow(key, window, limit), nil
	}
	l.degraded.Store(false)
	arr, ok := res.([]any)
	if !ok || len(arr) != 2 {
		l.degraded.Store(true)
		return l.fallback.allow(key, window, limit), nil
	}
	allowed, _ := arr[0].(int64)
	remaining, _ := arr[1].(int64)
	d := Decision{
		Allowed:   allowed == 1,
		Limit:     limit,
		Remaining: int(remaining),
		ResetUnix: now.Add(window).Unix(),
	}
	if !d.Allowed {
		d.RetryAfter = l.retryAfter(ctx, key, nowMs, window)
	}
	return d, nil
}

// retryAfter 估算最早一个窗口内请求过期所需时间（用于 Retry-After 头）。
func (l *redisLimiter) retryAfter(ctx context.Context, key string, nowMs int64, window time.Duration) time.Duration {
	oldest, err := l.rdb.ZRangeWithScores(ctx, key, 0, 0).Result()
	if err != nil || len(oldest) == 0 {
		return window
	}
	ms := int64(oldest[0].Score) + window.Milliseconds() - nowMs
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func (l *redisLimiter) Degraded() bool { return l.degraded.Load() }

func (l *redisLimiter) Close() error {
	if l.fallback != nil {
		l.fallback.close()
	}
	return nil
}
