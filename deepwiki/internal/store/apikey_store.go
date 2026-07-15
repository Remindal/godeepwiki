package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// APIKey API 密钥记录（只存哈希；硬约束 #2：禁止明文进入 Postgres / etcd / 日志）。
type APIKey struct {
	KeyID     string     // key_ + ULID
	Name      string
	KeyHash   string     // SHA-256(salt ‖ key) 十六进制
	Salt      string
	IsAdmin   bool
	RevokedAt *time.Time // nil = 未吊销
	CreatedAt time.Time
}

// APIKeyStore API 密钥仓储（变更总纲 §4.1）。
type APIKeyStore interface {
	// Upsert 启动引导：把 DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY 中的明文 key 哈希后幂等写入
	//（已存在同 key_hash 则跳过；salt 每 key 随机生成）。
	Upsert(ctx context.Context, k *APIKey) error
	// GetByHash 认证二级查找的 Postgres 端（Redis 缓存 auth:key:<sha256(key)> TTL 60s 在前）。
	GetByHash(ctx context.Context, keyHash string) (*APIKey, error)
	// Revoke 吊销：revoked_at = now()；调用方负责 DEL Redis 缓存 auth:key:<sha256(key)>。
	Revoke(ctx context.Context, keyID string) error
	// Count 启动时判断是否开发模式（0 = 跳过鉴权并 WARN，语义与基线一致）。
	Count(ctx context.Context) (int64, error)
}

type pgAPIKeyStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewAPIKeyStore(db *DB, logger *zap.Logger) APIKeyStore {
	return &pgAPIKeyStore{pool: db.Pool(), logger: logger}
}

var _ APIKeyStore = (*pgAPIKeyStore)(nil)

func (s *pgAPIKeyStore) Upsert(ctx context.Context, k *APIKey) error {
	// TODO: INSERT INTO api_keys (key_id, name, key_hash, salt, is_admin) VALUES ($1,$2,$3,$4,$5)
	// ON CONFLICT (key_hash) DO NOTHING（幂等）；全部参数化（硬约束 #11）；
	// 禁止记录明文 key 到日志（硬约束 #2）。
	panic("TODO: pgAPIKeyStore.Upsert not implemented")
}

func (s *pgAPIKeyStore) GetByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	// TODO: SELECT ... WHERE key_hash=$1 AND revoked_at IS NULL；pgx.ErrNoRows 由上层映射 40101 unauthorized；
	// keyHash 本身是哈希值不是明文，允许作为查询参数，但仍禁止明文 key 入日志（硬约束 #2）。
	panic("TODO: pgAPIKeyStore.GetByHash not implemented")
}

func (s *pgAPIKeyStore) Revoke(ctx context.Context, keyID string) error {
	// TODO: UPDATE api_keys SET revoked_at=now() WHERE key_id=$1 AND revoked_at IS NULL；
	// keyID 必须先过 key_ 前缀 + ULID 正则（硬约束 #11）。
	panic("TODO: pgAPIKeyStore.Revoke not implemented")
}

func (s *pgAPIKeyStore) Count(ctx context.Context) (int64, error) {
	// TODO: SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL。
	panic("TODO: pgAPIKeyStore.Count not implemented")
}
