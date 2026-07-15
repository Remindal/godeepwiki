// Package middleware Gin 中间件链：RequestID → Recovery → CORS → Auth → RateLimit。
package middleware

import (
	crand "crypto/rand"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ContextKeyRequestID gin.Context 中 request_id 的键。
const ContextKeyRequestID = "request_id"

// RequestID 生成或透传请求 ID（req_ + ULID；ULID 字典序与时间序一致，利索引与排序，§5.6）。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = "req_" + ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(crand.Reader, 0)).String()
		}
		c.Set(ContextKeyRequestID, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

// GetRequestID 从 gin.Context 取 request_id（handler 装配信封用）。
func GetRequestID(c *gin.Context) string {
	return c.GetString(ContextKeyRequestID)
}

// NewULID 生成带类型前缀的 ID（tsk_/repo_/chk_；§5.6）。各模块实现阶段统一使用本函数。
func NewULID(prefix string) string {
	return prefix + ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(crand.Reader, 0)).String()
}
