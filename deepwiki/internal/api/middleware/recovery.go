package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// Recovery panic 恢复：原始错误（含堆栈）只进 zap 日志，响应为脱敏固定文案（硬约束 #8）。
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("request_id", GetRequestID(c)),
					zap.String("path", c.FullPath()),
					zap.Stack("stack"),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, model.Envelope{
					Code:      model.CodeInternalError,
					Message:   model.MessageOf(model.CodeInternalError),
					RequestID: GetRequestID(c),
				})
			}
		}()
		c.Next()
	}
}
