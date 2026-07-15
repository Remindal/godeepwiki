// Package store PostgreSQL 存储层：全部状态持久化（硬约束 #3：Postgres tasks 表为任务状态唯一来源）。
package store

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// idRegex 校验统一 ID：前缀 + 26 位 Crockford Base32 ULID（硬约束 #11）。
var idRegex = regexp.MustCompile(`^(tsk_|repo_|chk_|key_)[0-9A-HJKMNP-TV-Z]{26}$`)

// validateID 校验 repo_id / task_id / chunk_id / key_id 格式。
func validateID(id string) error {
	if idRegex.MatchString(id) {
		return nil
	}
	return fmt.Errorf("invalid id format: %q", id)
}

// DB 连接封装（pgxpool 连接池）。
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// Open 建立 pgxpool 连接池（变更总纲 §4.1：MaxConns=10, MinConns=2, MaxConnLifetime=1h,
// HealthCheckPeriod=30s；DSN 仅由环境变量 DEEPWIKI_POSTGRES_DSN 注入，禁止 yaml 明文）。
// maxConns 来自 storage.postgres.max_conns（默认 10，可热更新后重建池）。
func Open(ctx context.Context, dsn string, maxConns int32, logger *zap.Logger) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres dsn empty: set DEEPWIKI_POSTGRES_DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	} else {
		cfg.MaxConns = 10
	}
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &DB{pool: pool, logger: logger}, nil
}

// Pool 暴露底层 *pgxpool.Pool（各 store 实现、VectorRetriever、health 探测用）。
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

// Ping 健康检查用（health 的 postgres.connected 字段）。
func (d *DB) Ping(ctx context.Context) error { return d.pool.Ping(ctx) }

// Stat 连接池统计（health 的 postgres.pool.total/idle 字段）。
func (d *DB) Stat() *pgxpool.Stat { return d.pool.Stat() }

// Close 优雅退出最后一步调用（硬约束 #10）。
func (d *DB) Close() { d.pool.Close() }

// WithTx 事务辅助：fn 返回错误即回滚，否则提交；panic 向上传播（回滚后由上层 recover，硬约束 #4）。
func (d *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Commit 成功后 Rollback 为 no-op；未提交则回滚，保证连接不泄漏（硬约束 #10）。
		_ = tx.Rollback(ctx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
