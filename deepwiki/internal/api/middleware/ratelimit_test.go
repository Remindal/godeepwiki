package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/ratelimit"
)

func TestRateLimiterMiddleware_429AndHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}
	defer rdb.Close()

	base := &config.Config{
		RateLimit: config.RateLimitConfig{
			PerIP:  config.PerIPConfig{RPS: 100, Burst: 100},
			PerKey: config.PerKeyConfig{IngestPerHour: 20, AskPerMinute: 1, WikiPerHour: 10},
		},
	}
	cm := config.NewManager(base, nil, 0, nil, zap.NewNop())
	limiter := ratelimit.NewRedisLimiter(rdb, zap.NewNop())
	defer limiter.Close()

	// 清理测试键。
	_ = rdb.Del(context.Background(), "rl:ip:127.0.0.1:ask").Err()

	rl := NewRateLimiter(cm, limiter, zap.NewNop())
	r := gin.New()
	r.POST("/api/v1/ask", rl.Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	// 第一次放行。
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/v1/ask", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request want 200 got %d body=%s", w1.Code, w1.Body.String())
	}
	if w1.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit")
	}

	// 第二次触发 L2 ask 限流（ask_per_minute=1）。
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/v1/ask", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request want 429 got %d body=%s", w2.Code, w2.Body.String())
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if w2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("X-RateLimit-Remaining want 0 got %s", w2.Header().Get("X-RateLimit-Remaining"))
	}
	if w2.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("missing X-RateLimit-Reset")
	}
}
