// Package task 统一任务系统：ingest / refresh / wiki 三类任务共用同一套
// 任务模型、状态机、Worker Pool、查询/取消端点（基线 §4）。
// 存储实现为 Postgres tasks 表（总纲 R1：pgx/v5 + pgxpool，参数化 $n 占位，硬约束 #11）。
package task

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// TaskStore 任务持久化（基线 §7，冻结签名；硬约束 #3：Postgres tasks 表为任务状态唯一来源，
// 禁止用内存 map 作为唯一存储；内存中只允许保存运行中任务的 context.Context 与 cancel 函数）。
type TaskStore interface {
	Create(ctx context.Context, t *model.Task) error
	Get(ctx context.Context, taskID string) (*model.Task, error)
	// UpdateState 内置状态机转移校验（model.CanTransition）；非法转移返回 40902。
	UpdateState(ctx context.Context, taskID string, patch model.TaskPatch) error
	List(ctx context.Context, f model.TaskFilter) (tasks []*model.Task, total int64, err error)
	SetCancelFlag(ctx context.Context, taskID string) error
	// FindInterrupted 启动恢复用：查出全部非终态任务（§4.6，Reconciler 数据源）。
	FindInterrupted(ctx context.Context) ([]*model.Task, error)
}

// postgresTaskStore tasks 表 Postgres 访问实现（pgxpool 连接池；$n 占位参数化，硬约束 #11）。
type postgresTaskStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewTaskStore 返回具体类型指针：同时满足 TaskStore（冻结接口）与 queue.RecoveryStore
//（ResetStaleRunning 为启动恢复扩展方法，不进入冻结接口）。
func NewTaskStore(pool *pgxpool.Pool, logger *zap.Logger) *postgresTaskStore {
	return &postgresTaskStore{pool: pool, logger: logger}
}

var _ TaskStore = (*postgresTaskStore)(nil)

func (s *postgresTaskStore) Create(ctx context.Context, t *model.Task) error {
	// TODO: INSERT INTO tasks 全字段（progress/stats/error/request_payload 序列化为 JSONB）；
	// 参数化 $1..$n（硬约束 #11）；created_at 由调用方给 UTC 时间写入 timestamptz 列，
	// 禁止数据库本地时区（硬约束 #13：全链路 UTC + RFC3339）。
	panic("TODO: postgresTaskStore.Create not implemented")
}

func (s *postgresTaskStore) Get(ctx context.Context, taskID string) (*model.Task, error) {
	// TODO: 主键查询（QueryRow + Scan JSONB 反序列化）；pgx.ErrNoRows 映射 model.ErrTaskNotFound。
	panic("TODO: postgresTaskStore.Get not implemented")
}

func (s *postgresTaskStore) UpdateState(ctx context.Context, taskID string, patch model.TaskPatch) error {
	// TODO: 按 patch 非 nil 字段动态 UPDATE（参数化 $n），要求（§4.3 状态转移规则，冻结）：
	// ① patch.State != nil 时先读当前 state，用 model.CanTransition 校验，非法返回 model.ErrInvalidTaskState；
	// ② 进入终态必须同时写 finished_at；③ 转入 failed 必须带 patch.Err（code/message/stage）；
	// ④ ClearErr=true 时 error_json 置 NULL；⑤ updated_at = now()（timestamptz，心跳语义：Reconciler 据此判僵死）。
	panic("TODO: postgresTaskStore.UpdateState not implemented")
}

func (s *postgresTaskStore) List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error) {
	// TODO: 按 type/state/repo_id 过滤（非空才加条件，参数化 $n）+ created_at DESC + 分页（§5.4）；
	// API 投影不含 cancel_flag/request_payload（model.Task 的 json tag 已保证）。
	panic("TODO: postgresTaskStore.List not implemented")
}

func (s *postgresTaskStore) SetCancelFlag(ctx context.Context, taskID string) error {
	// TODO: UPDATE tasks SET cancel_flag=true WHERE task_id=$1（§4.5 取消机制第一步）。
	panic("TODO: postgresTaskStore.SetCancelFlag not implemented")
}

func (s *postgresTaskStore) FindInterrupted(ctx context.Context) ([]*model.Task, error) {
	// TODO: 查出全部非终态任务（state NOT IN ('completed','failed','cancelled')），
	// 供 queue.Reconciler 重投（pending）或重置（running 僵死）（总纲 §4.3）。
	// 本骨架阶段返回 (nil, nil) 占位，下一轮改为真实查询。
	return nil, nil
}

// ResetStaleRunning 启动恢复扩展（queue.RecoveryStore 契约）：running 态且 updated_at 早于
// staleBefore（默认 5 分钟无心跳）→ 重置 pending（幂等 CAS：WHERE state NOT IN 终态，硬约束 #18）。
func (s *postgresTaskStore) ResetStaleRunning(ctx context.Context, staleBefore time.Time) (int64, error) {
	// TODO: UPDATE tasks SET state='pending', updated_at=now()
	//       WHERE state NOT IN ('completed','failed','cancelled','pending') AND updated_at < $1；
	// 返回 RowsAffected。骨架阶段返回 (0, nil) 占位。
	return 0, nil
}
