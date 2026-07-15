package store

import (
	"errors"
	"fmt"
	"strings"

	"deepwiki/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx" // pgx driver（连接串 scheme: pgx://）
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"
)

// Migrate 启动时执行全部未应用的迁移（变更总纲 §4.1：golang-migrate + iofs source，只前进原则不变）。
// 任一失败返回错误，启动方必须 panic 退出（启动失败优于带病运行）；
// dirty 状态直接 panic 并提示用 `migrate force <version>` 修复后重启。
func Migrate(dsn string, logger *zap.Logger) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrate iofs source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toPgxURL(dsn))
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("schema up to date")
			return nil
		}
		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			// 库处于 dirty 状态：人工核对后执行 `migrate force <version>` 再重启；禁止自动 force。
			panic(fmt.Sprintf("database schema dirty at version %d; run `migrate force %d` after manual verification, then restart", dirty.Version, dirty.Version))
		}
		return fmt.Errorf("migrate up: %w", err)
	}
	version, dirtyFlag, err := m.Version()
	if err != nil {
		return fmt.Errorf("migrate version: %w", err)
	}
	logger.Info("migrations applied", zap.Uint("version", version), zap.Bool("dirty", dirtyFlag))
	return nil
}

// toPgxURL 把 postgres:// 或 postgresql:// 连接串转为 golang-migrate pgx driver 的 pgx:// scheme；
// 已是 pgx:// 或其他 scheme（如含查询参数的标准串）原样返回。
func toPgxURL(dsn string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(dsn, p) {
			return "pgx://" + strings.TrimPrefix(dsn, p)
		}
	}
	return dsn
}
