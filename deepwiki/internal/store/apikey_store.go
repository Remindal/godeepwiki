package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	// FindByKey 按明文 key 查找：salt 每 key 随机，须逐条比对 SHA-256(salt‖key)（总纲 R14）。
	// 未命中或已吊销返回 (nil, nil)。
	FindByKey(ctx context.Context, key string) (*APIKey, error)
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
	if err := validateID(k.KeyID); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (key_id, name, key_hash, salt, is_admin, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key_hash) DO NOTHING
	`, k.KeyID, k.Name, k.KeyHash, k.Salt, k.IsAdmin, now)
	return err
}

func (s *pgAPIKeyStore) FindByKey(ctx context.Context, key string) (*APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT key_id, name, key_hash, salt, is_admin, revoked_at, created_at
		FROM api_keys
		WHERE revoked_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.KeyID, &k.Name, &k.KeyHash, &k.Salt, &k.IsAdmin, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(k.Salt + key))
		if hex.EncodeToString(sum[:]) == k.KeyHash {
			return &k, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *pgAPIKeyStore) Revoke(ctx context.Context, keyID string) error {
	if err := validateID(keyID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE api_keys SET revoked_at = now() WHERE key_id = $1 AND revoked_at IS NULL
	`, keyID)
	return err
}

func (s *pgAPIKeyStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL`).Scan(&n)
	return n, err
}
