package task

import (
	"context"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// workerPool 有界 Worker 池（硬约束 #6 并发上限：禁止无限制 go func 起后台任务；
// 池容量 = worker.pool_size，默认 2；RabbitMQ prefetch 与之相等，背压由 broker 侧 x-max-length 承担）。
// 原进程内有界队列（互斥锁+条件变量+slice）已整体移除，队列语义由 RabbitMQ 主队列承担。
type workerPool struct {
	size   int
	busy   atomic.Int64
	wg     sync.WaitGroup
	logger *zap.Logger
	// TODO（下一轮）：软扩缩容控制结构（worker 退出信号、resize 通道）。
}

func newWorkerPool(size int, logger *zap.Logger) *workerPool {
	return &workerPool{size: size, logger: logger}
}

// Run 启动 size 个 worker goroutine 消费 deliveries 并把每条消息交给 handle。
// handle 返回 queue 包约定的处理结果语义：ack（终态落库）/ nack requeue=false（进 DLX 重试链）/
// nack requeue=true（优雅退出让渡）。要求（硬约束 #4 并发安全）：
//  ① 每个 worker goroutine 必须 defer recover()：panic → 当前消息 nack requeue=false + 堆栈入日志，worker 继续存活；
//  ② handle 的 ctx 由 pool 派生（ctx 取消 → worker 退出循环，wg.Done）；
//  ③ busy 计数原子增减（health 的 worker.busy 字段）；
//  ④ 禁止在 handle 内再 go func 派生无约束 goroutine。
func (p *workerPool) Run(ctx context.Context, deliveries <-chan amqp.Delivery, handle func(ctx context.Context, d amqp.Delivery)) {
	// TODO: 按上述要求实现 worker 循环。骨架阶段不启动任何 goroutine（no-op）。
}

// Busy 运行中 worker 数（health 的 busy 字段）。
func (p *workerPool) Busy() int {
	// TODO: 返回 int(p.busy.Load())。骨架阶段返回 0。
	return 0
}

// Resize 热扩缩容（config 热更新订阅者调用，§8.2）：
// 扩容即起新 worker（注意与 consumer prefetch 联动调大）；缩容为软缩容——多余 worker 停止取新消息，
// 手头任务完成后自然退出（瞬态超调属预期取舍，§4.4 备注）。
func (p *workerPool) Resize(n int) {
	// TODO: 实现软扩缩容。骨架阶段 no-op。
}
