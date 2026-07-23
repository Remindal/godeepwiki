package task

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// workerPool 有界 Worker 池（硬约束 #6 并发上限：禁止无限制 go func 起后台任务；
// 池容量 = worker.pool_size，默认 2；RabbitMQ prefetch 与之相等，背压由 broker 侧 x-max-length 承担）。
// 原进程内有界队列（互斥锁+条件变量+slice）已整体移除，队列语义由 RabbitMQ 主队列承担。
type workerPool struct {
	size    atomic.Int64
	running atomic.Int64
	busy    atomic.Int64
	wg      sync.WaitGroup
	logger  *zap.Logger
	mu      sync.Mutex
	ctx     context.Context
	deliveries <-chan amqp.Delivery
	handle  func(ctx context.Context, d amqp.Delivery)
}

func newWorkerPool(size int, logger *zap.Logger) *workerPool {
	p := &workerPool{logger: logger}
	p.size.Store(int64(size))
	return p
}

// Run 启动 size 个 worker goroutine 消费 deliveries 并把每条消息交给 handle。
// handle 返回 queue 包约定的处理结果语义：ack（终态落库）/ nack requeue=false（进 DLX 重试链）/
// nack requeue=true（优雅退出让渡）。要求（硬约束 #4 并发安全）：
//  ① 每个 worker goroutine 必须 defer recover()：panic → 当前消息 nack requeue=false + 堆栈入日志，worker 继续存活；
//  ② handle 的 ctx 由 pool 派生（ctx 取消 → worker 退出循环，wg.Done）；
//  ③ busy 计数原子增减（health 的 worker.busy 字段）；
//  ④ 禁止在 handle 内再 go func 派生无约束 goroutine。
func (p *workerPool) Run(ctx context.Context, deliveries <-chan amqp.Delivery, handle func(ctx context.Context, d amqp.Delivery)) {
	p.mu.Lock()
	p.ctx = ctx
	p.deliveries = deliveries
	p.handle = handle
	size := 0
	if ctx.Err() == nil {
		size = int(p.size.Load())
	}
	p.mu.Unlock()
	if size <= 0 {
		return
	}

	for i := 0; i < size; i++ {
		p.wg.Add(1)
		p.running.Add(1)
		go p.worker(ctx, deliveries, handle)
	}
}

func (p *workerPool) worker(ctx context.Context, deliveries <-chan amqp.Delivery, handle func(ctx context.Context, d amqp.Delivery)) {
	defer p.wg.Done()
	defer p.running.Add(-1)

	for {
		// 软缩容：如果当前运行数超过目标容量，worker 主动退出。
		if p.running.Load() > p.size.Load() {
			return
		}

		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			p.handleOne(ctx, d, handle)
		}
	}
}

func (p *workerPool) handleOne(ctx context.Context, d amqp.Delivery, handle func(ctx context.Context, d amqp.Delivery)) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("worker panic recovered",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())),
				zap.String("task_id", taskIDFromDelivery(d)),
			)
			_ = d.Nack(false, false)
		}
	}()

	p.busy.Add(1)
	defer p.busy.Add(-1)
	handle(ctx, d)
}

func taskIDFromDelivery(d amqp.Delivery) string {
	// 尽力提取 task_id 用于日志，失败不影响流程。
	return fmt.Sprintf("%v", d.MessageId)
}

// Busy 运行中 worker 数（health 的 busy 字段）。
func (p *workerPool) Busy() int {
	return int(p.busy.Load())
}

// Wait 阻塞等待全部 worker 退出。
func (p *workerPool) Wait() {
	p.wg.Wait()
}

// Resize 热扩缩容（config 热更新订阅者调用，§8.2）：
// 扩容即起新 worker（注意与 consumer prefetch 联动调大）；缩容为软缩容——多余 worker 停止取新消息，
// 手头任务完成后自然退出（瞬态超调属预期取舍，§4.4 备注）。
func (p *workerPool) Resize(n int) {
	if n < 1 {
		n = 1
	}
	old := int(p.size.Load())
	p.size.Store(int64(n))

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx == nil || p.ctx.Err() != nil || p.deliveries == nil || p.handle == nil {
		return
	}
	for i := old; i < n; i++ {
		p.wg.Add(1)
		p.running.Add(1)
		go p.worker(p.ctx, p.deliveries, p.handle)
	}
}
