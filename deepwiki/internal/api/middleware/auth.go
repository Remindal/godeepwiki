package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
)

// ContextKeyAPIKey gin.Context 中已鉴权 API key 记录的键（*keyRecord；日志/审计使用时必须脱敏，硬约束 #2）。
const ContextKeyAPIKey = "api_key"

// keyRecord 缓存值结构（Redis 键 auth:key:<sha256(key)> → 本结构 JSON，TTL 60s，总纲 §4.4）。
type keyRecord struct {
	KeyID   string `json:"key_id"`
	IsAdmin bool   `json:"is_admin"`
	Revoked bool   `json:"revoked"`
}

// Auth X-API-Key 鉴权（总纲 R14/§4.4；硬约束 #2：密钥只存 SHA-256(salt‖key) 哈希，
// 禁止明文入 Postgres/etcd/日志）。devMode=true（auth.api_keys 为空）时跳过鉴权并打一次 WARN。
func Auth(cache redis.UniversalClient, keys store.APIKeyStore, devMode bool, logger *zap.Logger) gin.HandlerFunc {
	var warnOnce sync.Once
	return func(c *gin.Context) {
		if devMode {
			warnOnce.Do(func() { logger.Warn("auth disabled: auth.api_keys is empty (dev mode)") })
			c.Next()
			return
		}
		if c.FullPath() == "/api/v1/health" { // health 与 /metrics 免鉴权（§5.7）
			c.Next()
			return
		}
		// TODO: 实现二级查找（总纲 R14）：
		// ① key := c.GetHeader("X-API-Key")；空 → 40101；
		// ② sum := sha256(key) → GET auth:key:<hex(sum)（Redis 缓存，TTL 60s）；
		// ③ 缓存未命中 → keys.FindByHash(ctx, sum) 查 Postgres api_keys 表
		//    （逐条比对 SHA-256(salt‖key)；命中且未吊销 → 回写缓存 {key_id,is_admin,revoked:false}）；
		// ④ 未命中或已吊销 → 401 + 40101；命中 → c.Set(ContextKeyAPIKey, &rec) 放行；
		// ⑤ 吊销路径（管理端）必须同时 DEL auth:key:<sha256> 主动失效缓存；
		// ⑥ Redis 不可用 → 降级直查 Postgres 并记 WARN（可用性优先，不因此拒绝合法请求）。
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.Envelope{
				Code:      model.CodeUnauthorized,
				Message:   model.MessageOf(model.CodeUnauthorized),
				RequestID: GetRequestID(c),
			})
			return
		}
		c.Next()
	}
}

// AdminOnly PUT /api/v1/config 的 admin 鉴权（已鉴权 key 的 is_admin=false → 40301，§5.7）；
// 开发模式（无 key 记录）放行。
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		rec, _ := c.Get(ContextKeyAPIKey)
		if rec == nil { // 开发模式（无 key 配置）：放行
			c.Next()
			return
		}
		// TODO: kr := rec.(*keyRecord)；kr.IsAdmin → 放行；否则 403 + 40301。
		c.Next()
	}
}
