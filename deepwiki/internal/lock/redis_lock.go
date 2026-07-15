// Package lock Redis 分布式锁（总纲 R13：替代多 worker 场景下失效的 v1 原方案进程内去重机制）。
// 锁键：lock:refresh:<repo_id>；TTL 300s（持锁方 pipeline 正常短于 5 分钟，
// 不引入 watchdog：超时自动释放 + 任务 CAS 兜底，硬约束 #18）。
package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// refreshLockTTL refresh 互斥锁 TTL（300s，总纲 §4.4）。
const refreshLockTTL = 300 * time.Second

// unlockLua 解锁脚本（校验 owner token 后 DEL，防止误删他人锁；与总纲 §4.4 语义一致）：
const unlockLua = `
-- KEYS[1]=锁键  ARGV[1]=owner token
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`

// Locker refresh 互斥分布式锁。
type Locker struct {
	rdb    redis.UniversalClient
	unlock *redis.Script
	logger *zap.Logger
}

func New(rdb redis.UniversalClient, logger *zap.Logger) *Locker {
	return &Locker{rdb: rdb, unlock: redis.NewScript(unlockLua), logger: logger}
}

// AcquireRefresh 获取同仓 refresh 互斥锁：SET lock:refresh:<repo_id> <token> NX PX 300000；
// ok=false 表示他节点持锁（调用方映射 40902）；token 为 ULID（解锁时校验）。
func (l *Locker) AcquireRefresh(ctx context.Context, repoID string) (token string, ok bool, err error) {
	token = randomToken()
	key := "lock:refresh:" + repoID
	ok, err = l.rdb.SetNX(ctx, key, token, refreshLockTTL).Result()
	if err != nil {
		return "", false, fmt.Errorf("lock acquire: %w", err)
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

// ReleaseRefresh 释放锁：Lua 校验 token 后 DEL（仅持锁本人可解）。
func (l *Locker) ReleaseRefresh(ctx context.Context, repoID, token string) error {
	key := "lock:refresh:" + repoID
	_, err := l.unlock.Run(ctx, l.rdb, []string{key}, token).Result()
	if err != nil {
		return fmt.Errorf("lock release: %w", err)
	}
	return nil
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
