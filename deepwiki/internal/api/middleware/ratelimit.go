package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/ratelimit"
)

// RateLimiter 两级限流中间件（§9.1，硬约束 #1：禁止全局单桶）：
// L1 per-IP 滑动窗口（per_ip.rps/per_ip.burst，默认 10/20，作用于全部 /api/v1/*）；
// L2 per-API-key 配额（ingest_per_hour=20 / ask_per_minute=30 / wiki_per_hour=10，
// 作用于建任务类与问答类昂贵端点）。数值冻结（总纲 §2.8），仅替换存储实现。
type RateLimiter struct {
	cfg     *config.Manager
	limiter ratelimit.Limiter // Redis Lua 滑动窗口 + 进程内 x/time/rate 降级兜底（§5.11.5）
	logger  *zap.Logger
}

func NewRateLimiter(cfg *config.Manager, limiter ratelimit.Limiter, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{cfg: cfg, limiter: limiter, logger: logger}
}

// Middleware 限流中间件。【骨架阶段直通】下一轮实现，要求（硬约束 #1、§9.1~9.2、总纲 §4.4）：
//  1. L1 per-IP：limiter.Allow(ctx, "rl:ip:"+ip, 60s, rps*60+burst)（窗口换算见总纲 §4.4：limit = rps*60 + burst；
//     仅在 gin.SetTrustedProxies 配置后才采信 X-Forwarded-For）；
//  2. L2 per-API-key：ingest/refresh 走 "rl:key:<key_hash>:ingest"（3600s/20）、ask/ask-stream 走
//     "rl:key:<key_hash>:ask"（60s/30）、wiki generate 走 "rl:key:<key_hash>:wiki"（3600s/10）；
//     无 API key（开发模式）时 L2 退化为按 IP 计数；
//  3. 命中 → 429 + 42901 rate_limited + Retry-After 头 + X-RateLimit-Limit/Remaining/Reset
//     （Reset 为 UTC epoch 秒，响应头契约冻结）；
//  4. 未命中的受限端点响应同样携带 X-RateLimit-* 三件套；
//  5. Redis 不可用时 ratelimit 包内部自动降级进程内 x/time/rate 兜底 + WARN +
//     指标 deepwiki_ratelimit_degraded_total++ + health redis.ratelimit_degraded=true（可用性优先的有意取舍）；
//  6. 配置热更新：订阅 ConfigManager 重建窗口参数（§8.2）。
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 按上述要求实现两级限流（当前为骨架直通）。
		c.Next()
	}
}
