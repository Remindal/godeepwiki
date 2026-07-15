package task

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/model"
	"deepwiki/internal/queue"
)

// TaskManager 任务门面（基线 §7，冻结签名）。
type TaskManager interface {
	// Submit 落库 + 投递；队列满返回 model.ErrQueueFull（映射 42902）；
	// 投递 confirm 失败返回 model.ErrQueueUnavailable（映射 50302，总纲 §6 新增码）。
	Submit(ctx context.Context, t *model.Task) error
	// Cancel 置 cancel_flag + cancel context；终态任务返回 model.ErrInvalidTaskState（映射 40902）。
	Cancel(ctx context.Context, taskID string) error
	Get(ctx context.Context, taskID string) (*model.Task, error)
	List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error)
	Stats() WorkerStats
}

// WorkerStats Worker 池实时状态（health 的 worker 字段，总纲 §7）：
// Queued 语义 = RabbitMQ 主队列深度（QueueDeclarePassive 读 Messages）。
type WorkerStats struct {
	Busy   int `json:"busy"`
	Total  int `json:"total"`
	Queued int `json:"queued"`
}

// Manager TaskManager 实现骨架。
type Manager struct {
	store     TaskStore
	bus       eventbus.EventBus
	publisher queue.Publisher // RabbitMQ 瘦消息投递（confirm+mandatory）
	executors map[model.TaskType]Executor
	pool      *workerPool
	logger    *zap.Logger
	poolSize  int
	queueSize int // x-max-length = worker.queue_size（默认 100，背压预检阈值）
	mu        sync.Mutex
	cancels   map[string]context.CancelFunc // 运行中任务的取消函数（内存仅持 ctx 句柄，状态以 Postgres 为准，硬约束 #3）
}

func NewManager(store TaskStore, bus eventbus.EventBus, publisher queue.Publisher, cfg config.WorkerConfig, logger *zap.Logger) *Manager {
	return &Manager{
		store:     store,
		bus:       bus,
		publisher: publisher,
		executors: make(map[model.TaskType]Executor),
		logger:    logger,
		poolSize:  cfg.PoolSize,
		queueSize: cfg.QueueSize,
		cancels:   make(map[string]context.CancelFunc),
	}
}

var _ TaskManager = (*Manager)(nil)

// RegisterExecutor 注册任务类型执行器（main 装配时调用；下一轮注册 ingest/refresh/wiki 三个实现）。
func (m *Manager) RegisterExecutor(e Executor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executors[e.Type()] = e
}

// Start 启动消费与 Worker Pool（main 在 Reconciler 恢复完成后调用）。骨架阶段 no-op，下一轮实现消费循环。
func (m *Manager) Start(ctx context.Context, consumer *queue.Consumer) {
	// TODO: 启动消费循环（总纲 §4.3）：
	// ① consumer.Deliveries(ctx)（prefetch=pool_size）→ m.pool.Run(ctx, deliveries, m.dispatch)；
	// ② dispatch：解析 TaskMessage → CAS 抢占任务（硬约束 #18，见 executor.go 注释）→ 先查 cancel_flag
	//    （已取消直接落 cancelled 并写 finished_at）→ 写 started_at → 按 type 路由 Executor；
	// ③ 逐阶段 UpdateState + EventBus.Publish（结构化字段，禁止拼字符串）；
	// ④ worker 内全部逻辑包裹 defer recover()：panic → nack requeue=false 进 DLX（硬约束 #4）；
	// ⑤ 进度落库节流：每 500ms 或每推进 5% 一次（二者先到为准，§4.4）；
	// ⑥ 禁止绕过本池 go func 起任务（硬约束 #6）。
}

// Stop 软缩容等待（硬约束 #10 优雅退出）：consumer.Stop 停拉新消息 → 等在途任务完成
//（上限 server.shutdown_timeout）→ 未完成者 nack requeue=true 让别的节点接走。骨架阶段 no-op。
func (m *Manager) Stop(ctx context.Context) {
	// TODO: ① 停拉新消息；② 等待 worker 排空或 ctx 超时；③ 在途未完成任务 nack requeue=true
	//（Reconciler/其他节点接走继续执行，任务不丢）；④ 禁止直接杀 goroutine（硬约束 #4/#10）。
}

func (m *Manager) Submit(ctx context.Context, t *model.Task) error {
	// TODO: 投递路径（总纲 §4.3，硬约束 #3/#6/#16）：
	// ① Postgres 事务内 INSERT tasks（state=pending，queue_position=当前队列深度+1）——状态唯一来源是 Postgres；
	// ② 事务提交后 m.publisher.QueueDepth 预检：深度 ≥ m.queueSize（x-max-length，默认 100）
	//    → 任务落库 failed（error.code=42902）→ 返回 model.ErrQueueFull（映射 429 + 42902 + Retry-After）；
	// ③ m.publisher.Publish 瘦消息（body={"task_id","type"}，mandatory=true + publisher confirm）；
	// ④ queue.ErrPublishFailed → 任务标记 failed（error.code=50302）→ 返回 model.ErrQueueUnavailable
	//    （映射 503 + 50302 queue_unavailable，总纲 §6 新增码）；
	// ⑤ 成功后经 EventBus 发布 task.state_changed（state=pending，含 queue_position；payload 字段冻结）。
	panic("TODO: Manager.Submit not implemented")
}

func (m *Manager) Cancel(ctx context.Context, taskID string) error {
	// TODO: §4.5 取消机制（冻结）：
	// ① 终态任务返回 model.ErrInvalidTaskState（→ 40902）；
	// ② 非终态：SetCancelFlag + 若运行中调 m.cancels[taskID]()；Worker 捕获 ctx.Err() 后落 cancelled；
	//    尚未被消费的 pending 消息被消费时 CAS 失败直接 ack 丢弃（硬约束 #18，天然安全）；
	// ③ API 层返回 202 + 当前 task 快照（state 可能尚未变 cancelled，前端经 SSE/WS 收终态事件）。
	panic("TODO: Manager.Cancel not implemented")
}

func (m *Manager) Get(ctx context.Context, taskID string) (*model.Task, error) {
	// TODO: 委托 m.store.Get。
	panic("TODO: Manager.Get not implemented")
}

func (m *Manager) List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error) {
	// TODO: 委托 m.store.List。
	panic("TODO: Manager.List not implemented")
}

// Stats 骨架：Total 取配置 pool_size；Busy/Queued 下一轮取 workerPool 实时值与
// RabbitMQ 主队列深度（publisher.QueueDepth；health 验收依赖本方法，总纲 §7 worker 字段）。
func (m *Manager) Stats() WorkerStats {
	// TODO: Busy=m.pool.Busy()，Queued=m.publisher.QueueDepth（失败记 WARN 按 0 返回，health 不因此 500）。
	return WorkerStats{Busy: 0, Total: m.poolSize, Queued: 0}
}
