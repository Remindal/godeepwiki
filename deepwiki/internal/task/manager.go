package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/lock"
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

// Manager TaskManager 实现。
type Manager struct {
	store      TaskStore
	bus        eventbus.EventBus
	publisher  queue.Publisher
	executors  map[model.TaskType]Executor
	pool       *workerPool
	logger     *zap.Logger
	poolSize   int
	queueSize  int
	maxRetries int
	locker     *lock.Locker

	mu           sync.Mutex
	cancels      map[string]context.CancelFunc
	inflight     map[uint64]amqp.Delivery
	poolCancel   context.CancelFunc
	started      bool
	refreshLocks map[string]string // task_id -> redis lock token
}

func NewManager(store TaskStore, bus eventbus.EventBus, publisher queue.Publisher, cfg config.WorkerConfig, logger *zap.Logger) *Manager {
	return &Manager{
		store:        store,
		bus:          bus,
		publisher:    publisher,
		executors:    make(map[model.TaskType]Executor),
		pool:         newWorkerPool(cfg.PoolSize, logger),
		logger:       logger,
		poolSize:     cfg.PoolSize,
		queueSize:    cfg.QueueSize,
		maxRetries:   3,
		cancels:      make(map[string]context.CancelFunc),
		inflight:     make(map[uint64]amqp.Delivery),
		refreshLocks: make(map[string]string),
	}
}

var _ TaskManager = (*Manager)(nil)

// WithLocker 注入 Redis 分布式锁（refresh 互斥用）。
func (m *Manager) WithLocker(l *lock.Locker) *Manager {
	m.locker = l
	return m
}

// WithMaxRetries 设置消费失败后的 DLX 重试次数（默认 3）。
func (m *Manager) WithMaxRetries(n int) *Manager {
	if n < 0 {
		n = 0
	}
	m.maxRetries = n
	return m
}

// RegisterExecutor 注册任务类型执行器（main 装配时调用；下一轮注册 ingest/refresh/wiki 三个实现）。
func (m *Manager) RegisterExecutor(e Executor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executors[e.Type()] = e
}

// Start 启动消费与 Worker Pool（main 在 Reconciler 恢复完成后调用）。
func (m *Manager) Start(ctx context.Context, consumer *queue.Consumer) {
	deliveries, err := consumer.Deliveries(ctx)
	if err != nil {
		m.logger.Error("task manager start consumer failed", zap.Error(err))
		return
	}

	poolCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.poolCancel = cancel
	m.started = true
	m.mu.Unlock()

	m.pool.Run(poolCtx, deliveries, m.dispatch)
	m.logger.Info("task manager started", zap.Int("pool_size", m.poolSize))
}

// Stop 软缩容等待（硬约束 #10 优雅退出）：consumer.Stop 停拉新消息 → 等在途任务完成
//（上限 server.shutdown_timeout）→ 未完成者 nack requeue=true 让别的节点接走。
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	cancel := m.poolCancel
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.pool.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("task manager stopped gracefully")
	case <-ctx.Done():
		m.nackInflight(true)
		if cancel != nil {
			cancel()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		m.logger.Warn("task manager stopped with timeout")
	}
}

func (m *Manager) Submit(ctx context.Context, t *model.Task) error {
	if t.TaskID == "" {
		t.TaskID = newTaskID()
	}
	if t.Type == "" {
		return model.NewAPIError(model.CodeInvalidParam, "task type is required")
	}

	if _, ok := firstStateOf(t.Type); !ok {
		return model.NewAPIError(model.CodeInvalidParam, fmt.Sprintf("invalid task type %q", t.Type))
	}

	// refresh 任务需先获取同仓互斥锁（总纲 §4.4）。
	if t.Type == model.TaskTypeRefresh {
		if m.locker == nil {
			return model.NewAPIError(model.CodeInternalError, "refresh locker not configured")
		}
		token, acquired, err := m.locker.AcquireRefresh(ctx, t.RepoID)
		if err != nil {
			return model.NewAPIError(model.CodeInternalError, "refresh lock acquire failed")
		}
		if !acquired {
			return model.ErrRepoAlreadyExists // 同仓 refresh 冲突，映射 40901。
		}
		m.mu.Lock()
		m.refreshLocks[t.TaskID] = token
		m.mu.Unlock()
	}

	// ① 预检队列深度（用于 queue_position 与背压）。
	depth, err := m.publisher.QueueDepth(ctx)
	if err != nil {
		m.logger.Warn("queue depth precheck failed", zap.Error(err))
		depth = 0
	}

	// ② 队列满：直接落库 failed(42902)。pending -> failed 不是合法状态转移，因此直接 INSERT 终态。
	if depth >= m.queueSize {
		t.State = model.TaskStateFailed
		t.QueuePosition = 0
		t.FinishedAt = ptrTime(time.Now().UTC())
		t.Err = &model.TaskError{Code: model.CodeQueueFull, Message: model.MessageOf(model.CodeQueueFull), Stage: string(model.TaskStatePending)}
		if err := m.store.Create(ctx, t); err != nil {
			return err
		}
		return model.ErrQueueFull
	}

	// ③ 正常落库 pending。
	t.State = model.TaskStatePending
	t.QueuePosition = depth + 1
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	if err := m.store.Create(ctx, t); err != nil {
		return err
	}

	// ④ 投递瘦消息。
	msg := queue.TaskMessage{TaskID: t.TaskID, Type: string(t.Type)}
	if err := m.publisher.Publish(ctx, msg); err != nil {
		// 投递失败：任务落 failed(50302)。
		if ext, ok := m.store.(storeExt); ok {
			_ = ext.FailTask(ctx, t.TaskID, &model.TaskError{
				Code:    model.CodeQueueUnavailable,
				Message: model.MessageOf(model.CodeQueueUnavailable),
				Stage:   string(model.TaskStatePending),
			})
		}
		return model.ErrQueueUnavailable
	}

	// ⑤ 发布 state_changed 事件。
	m.publishStateChanged(ctx, t)
	return nil
}

func (m *Manager) Cancel(ctx context.Context, taskID string) error {
	t, err := m.store.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if t.State.IsTerminal() {
		return model.ErrInvalidTaskState
	}

	if err := m.store.SetCancelFlag(ctx, taskID); err != nil {
		return err
	}

	// pending 任务直接置为 cancelled，避免后续消息消费时再执行。
	if t.State == model.TaskStatePending {
		now := time.Now().UTC()
		if err := m.store.UpdateState(ctx, taskID, model.TaskPatch{
			State:      ptrState(model.TaskStateCancelled),
			FinishedAt: &now,
		}); err != nil {
			return err
		}
	}

	m.mu.Lock()
	if cancel, ok := m.cancels[taskID]; ok {
		cancel()
	}
	m.mu.Unlock()

	return nil
}

func (m *Manager) Get(ctx context.Context, taskID string) (*model.Task, error) {
	return m.store.Get(ctx, taskID)
}

func (m *Manager) List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error) {
	return m.store.List(ctx, f)
}

// Stats 返回 Worker 池实时状态；QueueDepth 失败按 0 处理，health 不因此 500。
func (m *Manager) Stats() WorkerStats {
	busy := 0
	if m.pool != nil {
		busy = m.pool.Busy()
	}
	queued := 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if depth, err := m.publisher.QueueDepth(ctx); err != nil {
		m.logger.Warn("queue depth stats failed", zap.Error(err))
	} else {
		queued = depth
	}
	return WorkerStats{Busy: busy, Total: m.poolSize, Queued: queued}
}

// dispatch 是 worker pool 的消息处理回调（单条消息在一个 worker goroutine 内顺序执行）。
func (m *Manager) dispatch(ctx context.Context, d amqp.Delivery) {
	var msg queue.TaskMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil || msg.TaskID == "" {
		m.logger.Error("invalid task message", zap.Error(err), zap.ByteString("body", d.Body))
		_ = d.Nack(false, false)
		return
	}

	firstState, ok := firstStateOf(model.TaskType(msg.Type))
	if !ok {
		m.logger.Error("unknown task type in message", zap.String("type", msg.Type), zap.String("task_id", msg.TaskID))
		_ = d.Nack(false, false)
		return
	}

	ext, ok := m.store.(storeExt)
	if !ok {
		m.logger.Error("task store does not support CAS extensions")
		_ = d.Nack(false, false)
		return
	}

	// CAS 抢占：pending -> firstState。
	claimed, err := ext.ClaimPending(ctx, msg.TaskID, firstState)
	if err != nil {
		m.logger.Error("claim pending failed", zap.String("task_id", msg.TaskID), zap.Error(err))
		_ = d.Nack(false, false)
		return
	}
	if !claimed {
		// 已被抢占、已取消或重复消费：幂等 ack 丢弃。
		_ = d.Ack(false)
		return
	}

	t, err := m.store.Get(ctx, msg.TaskID)
	if err != nil {
		if errors.Is(err, model.ErrTaskNotFound) {
			_ = d.Ack(false)
			return
		}
		m.resetAndRetry(ctx, d, msg, err)
		return
	}

	// 已取消任务：落 cancelled 并 ack。
	if t.CancelFlag {
		_ = m.finishTerminal(ctx, t.TaskID, model.TaskStateCancelled, nil)
		_ = d.Ack(false)
		return
	}

	m.mu.Lock()
	ex, ok := m.executors[t.Type]
	m.mu.Unlock()
	if !ok {
		m.logger.Error("no executor registered", zap.String("type", string(t.Type)), zap.String("task_id", t.TaskID))
		_ = ext.FailTask(ctx, t.TaskID, &model.TaskError{
			Code:    model.CodeInternalError,
			Message: model.MessageOf(model.CodeInternalError),
			Stage:   string(t.State),
		})
		_ = d.Ack(false)
		return
	}

	// 注册任务级 cancel 与在途跟踪。
	taskCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancels[t.TaskID] = cancel
	m.inflight[d.DeliveryTag] = d
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.cancels, t.TaskID)
		delete(m.inflight, d.DeliveryTag)
		m.mu.Unlock()
		cancel()
	}()

	runErr := ex.Execute(taskCtx, t)

	switch {
	case runErr == nil:
		m.releaseRefreshLock(ctx, t)
		_ = d.Ack(false)
	case errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded):
		// 被取消：确保落 cancelled（执行器可能已经落过，忽略状态机错误）。
		_ = m.finishTerminal(ctx, t.TaskID, model.TaskStateCancelled, nil)
		m.releaseRefreshLock(ctx, t)
		_ = d.Ack(false)
	default:
		// 执行器把业务失败自行落库 failed 后返回 nil；此处仅处理瞬时/基础设施错误，进入重试链。
		m.resetAndRetry(ctx, d, msg, runErr)
	}
}

// resetAndRetry 把任务复位 pending 并按重试次数进入 DLX 延迟队列；重试耗尽则落 failed(50003) 并进 DLQ。
func (m *Manager) resetAndRetry(ctx context.Context, d amqp.Delivery, msg queue.TaskMessage, cause error) {
	m.logger.Error("task execution transient error", zap.String("task_id", msg.TaskID), zap.Error(cause))

	ext, ok := m.store.(storeExt)
	if !ok {
		_ = d.Nack(false, false)
		return
	}

	attempt := retryCount(d)
	if attempt < m.maxRetries {
		if err := ext.ResetToPending(ctx, msg.TaskID); err != nil {
			m.logger.Error("reset to pending failed", zap.String("task_id", msg.TaskID), zap.Error(err))
			_ = d.Nack(false, false)
			return
		}
		if rp, ok := m.publisher.(queue.RetryPublisher); ok {
			if err := rp.PublishRetry(ctx, msg, attempt); err != nil {
				m.logger.Error("republish retry failed", zap.String("task_id", msg.TaskID), zap.Error(err))
				_ = d.Nack(false, false)
				return
			}
			_ = d.Ack(false)
			return
		}
	}

	// 重试耗尽或 publisher 不支持重试扩展：落 failed(50003) 并进 DLQ 审计。
	_ = ext.FailTask(ctx, msg.TaskID, &model.TaskError{
		Code:    50003,
		Message: "task retry exhausted",
		Stage:   string(model.TaskStateFailed),
	})
	if rp, ok := m.publisher.(queue.RetryPublisher); ok {
		_ = rp.PublishToDLQ(ctx, msg)
	}
	_ = d.Ack(false)
}

func (m *Manager) finishTerminal(ctx context.Context, taskID string, state model.TaskState, e *model.TaskError) error {
	now := time.Now().UTC()
	patch := model.TaskPatch{State: &state, FinishedAt: &now, ClearErr: true}
	if state == model.TaskStateFailed && e != nil {
		patch.Err = e
		patch.ClearErr = false
	}
	if err := m.store.UpdateState(ctx, taskID, patch); err != nil {
		if errors.Is(err, model.ErrInvalidTaskState) {
			// 已经是终态：幂等忽略。
			return nil
		}
		return err
	}
	return nil
}

func (m *Manager) releaseRefreshLock(ctx context.Context, t *model.Task) {
	if t.Type != model.TaskTypeRefresh || m.locker == nil {
		return
	}
	m.mu.Lock()
	token, ok := m.refreshLocks[t.TaskID]
	delete(m.refreshLocks, t.TaskID)
	m.mu.Unlock()
	if !ok {
		return
	}
	if err := m.locker.ReleaseRefresh(ctx, t.RepoID, token); err != nil {
		m.logger.Warn("release refresh lock failed", zap.String("task_id", t.TaskID), zap.Error(err))
	}
}

func (m *Manager) nackInflight(requeue bool) {
	m.mu.Lock()
	copyInflight := make(map[uint64]amqp.Delivery, len(m.inflight))
	for k, v := range m.inflight {
		copyInflight[k] = v
	}
	m.mu.Unlock()
	for _, d := range copyInflight {
		_ = d.Nack(false, requeue)
	}
}

func (m *Manager) publishStateChanged(ctx context.Context, t *model.Task) {
	payload := struct {
		State         model.TaskState `json:"state"`
		Progress      model.Progress  `json:"progress"`
		Stats         model.Stats     `json:"stats"`
		QueuePosition int             `json:"queue_position,omitempty"`
	}{
		State:         t.State,
		Progress:      t.Progress,
		Stats:         t.Stats,
		QueuePosition: t.QueuePosition,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		m.logger.Warn("marshal state_changed payload failed", zap.Error(err))
		return
	}
	ev := model.Event{
		Type:      model.EventTypeTaskStateChanged,
		RepoID:    t.RepoID,
		TaskID:    t.TaskID,
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(b),
	}
	if err := m.bus.Publish(ctx, ev); err != nil {
		m.logger.Warn("publish state_changed event failed", zap.Error(err))
	}
}

// storeExt 是 TaskStore 之上的内部扩展（不污染冻结接口），由 postgresTaskStore 实现。
type storeExt interface {
	ClaimPending(ctx context.Context, taskID string, toState model.TaskState) (bool, error)
	ResetToPending(ctx context.Context, taskID string) error
	FailTask(ctx context.Context, taskID string, e *model.TaskError) error
}

func firstStateOf(tt model.TaskType) (model.TaskState, bool) {
	switch tt {
	case model.TaskTypeIngest:
		return model.TaskStateCloning, true
	case model.TaskTypeRefresh:
		return model.TaskStateFetching, true
	case model.TaskTypeWiki:
		return model.TaskStateOutlining, true
	default:
		return "", false
	}
}

func retryCount(d amqp.Delivery) int {
	v, ok := d.Headers["x-retry-count"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	default:
		return 0
	}
}

func newTaskID() string {
	// 骨架阶段由调用方生成 task_id；此兜底仅用于测试/异常路径。
	return fmt.Sprintf("tsk_%d", time.Now().UnixNano())
}

func ptrState(s model.TaskState) *model.TaskState { return &s }
func ptrTime(t time.Time) *time.Time              { return &t }
