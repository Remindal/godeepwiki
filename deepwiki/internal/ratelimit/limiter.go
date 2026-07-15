// Package ratelimit 分布式限流：Redis Lua 滑动窗口（主实现）+ 进程内 x/time/rate（降级兜底）
//（总纲 §4.4 / R11，硬约束 #1：per-IP + per-API-key 两级，禁止全局单桶；语义与数值冻结）。
package ratelimit

import (
	"context"
	"time"
)

// Decision 一次限流判定结果（响应头 X-RateLimit-* 与 Retry-After 的数据源，契约冻结）。
type Decision struct {
	Allowed    bool          // 是否放行
	Limit      int           // 窗口配额（X-RateLimit-Limit）
	Remaining  int           // 剩余额度（X-RateLimit-Remaining）
	ResetUnix  int64         // 窗口重置时刻（UTC epoch 秒，X-RateLimit-Reset）
	RetryAfter time.Duration // 命中限流时的 Retry-After 秒数
	Degraded   bool          // true = Redis 不可用已降级进程内兜底（health redis.ratelimit_degraded）
}

// Limiter 限流器接口（中间件只依赖本接口，不感知 Redis）。
type Limiter interface {
	// Allow 在窗口 window 内对 key 计数，超过 limit 返回 Allowed=false。
	// key 形态（总纲 §4.4，逐字一致）：
	//   rl:ip:<ip>                    —— L1 per-IP（window 60s，limit = rps*60 + burst）
	//   rl:key:<key_hash>:ingest      —— L2 摄取/刷新（3600s/20）
	//   rl:key:<key_hash>:ask         —— L2 问答（60s/30）
	//   rl:key:<key_hash>:wiki        —— L2 wiki 生成（3600s/10）
	Allow(ctx context.Context, key string, window time.Duration, limit int) (Decision, error)
	// Degraded 当前是否处于降级态（health 探测循环读取）。
	Degraded() bool
	// Close 释放资源（进程内兜底表的清理 goroutine 退出）。
	Close() error
}
