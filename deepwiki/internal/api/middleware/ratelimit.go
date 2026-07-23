package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
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

type rateLimitCategory int

const (
	categoryNone rateLimitCategory = iota
	categoryIngest
	categoryAsk
	categoryWiki
)

func (c rateLimitCategory) String() string {
	switch c {
	case categoryIngest:
		return "ingest"
	case categoryAsk:
		return "ask"
	case categoryWiki:
		return "wiki"
	default:
		return ""
	}
}

func categoryOf(c *gin.Context) rateLimitCategory {
	if c.Request.Method != http.MethodPost {
		return categoryNone
	}
	switch c.FullPath() {
	case "/api/v1/ingest", "/api/v1/repos/:repo_id/refresh":
		return categoryIngest
	case "/api/v1/ask", "/api/v1/ask/stream":
		return categoryAsk
	case "/api/v1/wiki/generate":
		return categoryWiki
	default:
		return categoryNone
	}
}

func l2Quota(cfg *config.Config, cat rateLimitCategory) (time.Duration, int) {
	switch cat {
	case categoryIngest:
		return time.Hour, cfg.RateLimit.PerKey.IngestPerHour
	case categoryAsk:
		return time.Minute, cfg.RateLimit.PerKey.AskPerMinute
	case categoryWiki:
		return time.Hour, cfg.RateLimit.PerKey.WikiPerHour
	default:
		return time.Minute, 0
	}
}

func setRateLimitHeaders(c *gin.Context, d ratelimit.Decision) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(d.ResetUnix, 10))
}

func (l *RateLimiter) l2Key(c *gin.Context, ip string, cat rateLimitCategory) string {
	if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
		sum := sha256.Sum256([]byte(apiKey))
		return "rl:key:" + hex.EncodeToString(sum[:]) + ":" + cat.String()
	}
	// 开发模式无 API key：L2 退化为按 IP 计数。
	return "rl:ip:" + ip + ":" + cat.String()
}

func (l *RateLimiter) reject(c *gin.Context, d ratelimit.Decision) {
	setRateLimitHeaders(c, d)
	retry := int(d.RetryAfter / time.Second)
	if retry < 1 {
		retry = 1
	}
	c.Header("Retry-After", strconv.Itoa(retry))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, model.Envelope{
		Code:      model.CodeRateLimited,
		Message:   model.MessageOf(model.CodeRateLimited),
		RequestID: GetRequestID(c),
	})
}

// Middleware 限流中间件。
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := l.cfg.Get()
		ip := c.ClientIP()
		ctx := c.Request.Context()

		// L1 per-IP 滑动窗口：limit = rps*60 + burst，window=60s。
		l1Limit := cfg.RateLimit.PerIP.RPS*60 + cfg.RateLimit.PerIP.Burst
		l1, err := l.limiter.Allow(ctx, "rl:ip:"+ip, time.Minute, l1Limit)
		if err != nil {
			l.logger.Warn("ratelimit l1 error", zap.Error(err))
		}
		if !l1.Allowed {
			l.reject(c, l1)
			return
		}
		setRateLimitHeaders(c, l1)

		// L2 per-API-key 配额（仅昂贵端点）。
		if cat := categoryOf(c); cat != categoryNone {
			window, limit := l2Quota(cfg, cat)
			key := l.l2Key(c, ip, cat)
			d, err := l.limiter.Allow(ctx, key, window, limit)
			if err != nil {
				l.logger.Warn("ratelimit l2 error", zap.Error(err))
			}
			if !d.Allowed {
				l.reject(c, d)
				return
			}
			setRateLimitHeaders(c, d)
		}

		c.Next()
	}
}
