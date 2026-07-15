package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 仅放行 server.cors_allowed_origins 白名单（配置校验已拒绝 "*"，
// 此处再过滤一次作双保险，硬约束 #12）。
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			continue // 禁止通配
		}
		allow[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allow[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Request-ID, Last-Event-ID")
				c.Header("Access-Control-Max-Age", "86400")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent) // 预检直接应答，不进 Auth（§5.7）
			return
		}
		c.Next()
	}
}
