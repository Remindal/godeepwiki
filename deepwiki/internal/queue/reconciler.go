package queue

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// RecoveryStore Reconciler 所需的 tasks 表最小访问面（由 task 包的具体实现满足；
// 不污染冻结的 TaskStore 接口，见 §5.7 ⑤）。
type RecoveryStore interface {
	// FindInterrupted 查出全部非终态任务（state NOT IN ('completed','failed','cancelled')）。
	FindInterrupted(ctx context.Context) ([]*model.Task, error)
	// ResetStaleRunning running 态且 updated_at 早于 staleBefore（默认 now-5min，无心跳）
	// → 重置为 pending（幂等 CAS：WHERE state NOT IN 终态，硬约束 #18），返回重置行数。
	ResetStaleRunning(ctx context.Context, staleBefore time.Time) (int64, error)
}

// Reconciler 启动恢复器（总纲 §4.3）：worker 启动时扫描 Postgres 中非终态任务——
// pending 且无消费者持有 → 重新投递；running 态且 updated_at 超 5 分钟无心跳 → 重置 pending 重投。
// 幂等 CAS 保证多节点并发恢复安全（硬约束 #18）；worker 崩溃后 Reconciler 从 DB 重建队列视图，
// 消息系统挂一分钟任务一个都不丢（总纲 §10 答辩话术 1）。
type Reconciler struct {
	store      RecoveryStore
	pub        Publisher
	logger     *zap.Logger
	staleAfter time.Duration // 默认 5min
}

func NewReconciler(store RecoveryStore, pub Publisher, logger *zap.Logger) *Reconciler {
	return &Reconciler{store: store, pub: pub, logger: logger, staleAfter: 5 * time.Minute}
}

// StartPeriodic 周期恢复（main 以 goroutine 启动）：除启动恢复外，周期执行 Recover，
// worker 崩溃后 running 僵死任务不等重启即可被重置重投（总纲 §4.3 / §10 答辩话术 1）。
// interval 建议 1min（僵死判定 staleAfter=5min，周期远小于它即可）。
func (r *Reconciler) StartPeriodic(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				r.logger.Error("reconciler periodic panic recovered", zap.Any("panic", rec))
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.Recover(ctx); err != nil {
					r.logger.Error("reconciler periodic recover failed", zap.Error(err))
				}
			}
		}
	}()
}

// Recover 执行一次启动恢复（main 在 worker pool 启动前调用）。
func (r *Reconciler) Recover(ctx context.Context) error {
	reset, err := r.store.ResetStaleRunning(ctx, time.Now().UTC().Add(-r.staleAfter))
	if err != nil {
		return fmt.Errorf("reconciler reset stale running: %w", err)
	}

	tasks, err := r.store.FindInterrupted(ctx)
	if err != nil {
		return fmt.Errorf("reconciler find interrupted: %w", err)
	}

	republished := 0
	for _, t := range tasks {
		msg := TaskMessage{TaskID: t.TaskID, Type: string(t.Type)}
		if err := r.pub.Publish(ctx, msg); err != nil {
			r.logger.Error("reconciler republish failed",
				zap.String("task_id", t.TaskID),
				zap.Error(err),
			)
			continue
		}
		republished++
	}

	r.logger.Info("reconciler recovered",
		zap.Int64("reset_stale", reset),
		zap.Int("republished", republished),
		zap.Int("interrupted", len(tasks)),
	)
	return nil
}
