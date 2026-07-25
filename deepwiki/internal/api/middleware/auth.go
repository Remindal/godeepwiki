package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

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

// authCacheTTL API key 认证缓存 TTL（60s，总纲 §4.4）。
const authCacheTTL = 60 * time.Second

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

		key := c.GetHeader("X-API-Key")
		if key == "" {
			rejectUnauthorized(c)
			return
		}

		rec, ok := lookupKey(c, cache, keys, key, logger)
		if !ok {
			rejectUnauthorized(c)
			return
		}
		c.Set(ContextKeyAPIKey, rec)
		c.Next()
	}
}

// lookupKey 二级查找：Redis 缓存（auth:key:<sha256(key)>，TTL 60s）→ Postgres api_keys 表。
// Redis 不可用时降级直查 Postgres 并记 WARN（可用性优先，不因此拒绝合法请求）。
func lookupKey(c *gin.Context, cache redis.UniversalClient, keys store.APIKeyStore, key string, logger *zap.Logger) (*keyRecord, bool) {
	ctx := c.Request.Context()
	sum := sha256.Sum256([]byte(key))
	cacheKey := "auth:key:" + hex.EncodeToString(sum[:])

	// L1 Redis 缓存。
	if cache != nil {
		cached, err := cache.Get(ctx, cacheKey).Result()
		if err == nil {
			var rec keyRecord
			if json.Unmarshal([]byte(cached), &rec) == nil {
				if rec.Revoked {
					return nil, false
				}
				return &rec, true
			}
		} else if err != redis.Nil {
			// Redis 故障：降级直查 Postgres（可用性优先）。
			logger.Warn("auth cache unavailable, fallback to postgres", zap.Error(err))
			return lookupDB(ctx, keys, key, nil, logger)
		}
	}

	// L2 Postgres（命中回写缓存）。
	return lookupDB(ctx, keys, key, cache, logger)
}

// lookupDB 查 api_keys 表；cache 非 nil 时命中回写 auth:key:<sha256(key)>（TTL 60s）。
func lookupDB(ctx context.Context, keys store.APIKeyStore, key string, cache redis.UniversalClient, logger *zap.Logger) (*keyRecord, bool) {
	k, err := keys.FindByKey(ctx, key)
	if err != nil {
		logger.Error("auth db lookup failed", zap.Error(err))
		return nil, false
	}
	if k == nil { // 未命中或已吊销（GetByHash 语义：revoked_at IS NULL）
		return nil, false
	}
	rec := &keyRecord{KeyID: k.KeyID, IsAdmin: k.IsAdmin, Revoked: false}
	if cache != nil {
		sum := sha256.Sum256([]byte(key))
		if b, err := json.Marshal(rec); err == nil {
			if err := cache.Set(ctx, "auth:key:"+hex.EncodeToString(sum[:]), b, authCacheTTL).Err(); err != nil {
				logger.Warn("auth cache write failed", zap.Error(err))
			}
		}
	}
	return rec, true
}

func rejectUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, model.Envelope{
		Code:      model.CodeUnauthorized,
		Message:   model.MessageOf(model.CodeUnauthorized),
		RequestID: GetRequestID(c),
	})
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
		kr, ok := rec.(*keyRecord)
		if !ok || !kr.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, model.Envelope{
				Code:      model.CodeForbidden,
				Message:   model.MessageOf(model.CodeForbidden),
				RequestID: GetRequestID(c),
			})
			return
		}
		c.Next()
	}
}
