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
	// TODO: 实现判定，要求（总纲 §4.4）：
	// ① script.Run(ctx, l.rdb, []string{key}, now_ms, window.Milliseconds(), limit) 原子执行；
	// ② 成功 → 解析 {allowed, remaining} 装配 Decision（ResetUnix=now+window，命中时 RetryAfter≈窗口剩余，
	//    可由 PTTL 精确化）；degraded 置 false；
	// ③ Redis 错误（网络/超时/哨兵切换中）→ 降级 l.fallback.allow(key, window, limit) 放行判定 +
	//    WARN 日志 + degraded 置 true + 指标 deepwiki_ratelimit_degraded_total++
	//    （可用性优先的有意取舍，总纲 §4.4；恢复成功后 degraded 自动回落 false）；
	// ④ 指标 deepwiki_redis_op_duration_seconds{op="ratelimit_lua"} 计时。
	panic("TODO: redisLimiter.Allow not implemented")
}

func (l *redisLimiter) Degraded() bool { return l.degraded.Load() }

func (l *redisLimiter) Close() error {
	if l.fallback != nil {
		l.fallback.close()
	}
	return nil
}
