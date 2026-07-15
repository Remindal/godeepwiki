package queue

import (
	"context"
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
	store RecoveryStore
	pub   Publisher
	logger *zap.Logger
	staleAfter time.Duration // 默认 5min
}

func NewReconciler(store RecoveryStore, pub Publisher, logger *zap.Logger) *Reconciler {
	return &Reconciler{store: store, pub: pub, logger: logger, staleAfter: 5 * time.Minute}
}

// Recover 执行一次启动恢复（main 在 worker pool 启动前调用）。
func (r *Reconciler) Recover(ctx context.Context) error {
	// TODO: 实现启动恢复，要求：
	// ① ResetStaleRunning(time.Now().UTC().Add(-r.staleAfter))：僵死 running 任务重置 pending；
	// ② FindInterrupted 逐任务 publisher.Publish(TaskMessage{TaskID, Type}) 重投瘦消息；
	//    单条投递失败只记 ERROR 并继续（下一轮周期补偿或人工巡检 DLQ），禁止中断整个恢复流程；
	// ③ 恢复完成 logger.Info("reconciler recovered", zap.Int("republished", n))。
	panic("TODO: Reconciler.Recover not implemented")
}
