// Package queue RabbitMQ 任务队列：连接管理、拓扑声明、瘦消息投递与消费（总纲 §4.3）。
// 设计要点：RabbitMQ 只承担「任务投递与执行跨进程/跨节点」的传输职责，
// 任务状态唯一来源 = Postgres tasks 表（硬约束 #3）；消息为瘦消息（硬约束 #16）。
package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// 拓扑常量（总纲 §4.3，逐字一致，禁止改名）。
const (
	// ExchangeTasks direct exchange：任务消息经路由键 deepwiki.task.jobs 进入主队列。
	ExchangeTasks = "deepwiki.tasks"
	// QueueJobs 主队列：x-max-length = worker.queue_size（默认 100，硬约束 #6），
	// x-dead-letter-exchange = deepwiki.tasks.dlx（nack requeue=false 的消息进 DLX 重试链）。
	QueueJobs = "deepwiki.task.jobs"
	// ExchangeDLX 死信 exchange（direct）：重试队列与死信队列的汇聚点。
	ExchangeDLX = "deepwiki.tasks.dlx"
	// QueueDLQ 死信队列：重试耗尽（默认 3 次，queue.rabbitmq.max_retries）后的最终归宿；
	// DLQ 消息由 Reconciler/运维巡检消费并落库 failed/50003。
	QueueDLQ = "deepwiki.task.dlq"
	// 重试队列 TTL 链：TTL 到期后经 DLX 回流主队列，实现延迟重试（最多 3 次）。
	QueueRetry5s  = "deepwiki.task.retry.5s"  // x-message-ttl=5000
	QueueRetry30s = "deepwiki.task.retry.30s" // x-message-ttl=30000
	QueueRetry5m  = "deepwiki.task.retry.5m"  // x-message-ttl=300000
)

// TaskMessage 瘦消息协议（硬约束 #16：body ≤ 4KB，只携带 task_id+type；
// 禁止把任务状态/进度/请求大对象塞进消息——状态唯一来源 = Postgres tasks 表，硬约束 #3）。
type TaskMessage struct {
	TaskID string `json:"task_id"` // tsk_ + ULID(26)
	Type   string `json:"type"`    // ingest|refresh|wiki
}

// Conn RabbitMQ 连接封装（拓扑声明、通道工厂、优雅关闭）。
type Conn struct {
	conn       *amqp.Connection
	logger     *zap.Logger
	queueMaxLen int // x-max-length = worker.queue_size（默认 100）
}

// Dial 建立 RabbitMQ 连接（url 仅由环境变量 DEEPWIKI_RABBITMQ_URL 注入，禁止 yaml 明文，硬约束 #2）。
func Dial(ctx context.Context, url string, queueMaxLen int, logger *zap.Logger) (*Conn, error) {
	// TODO: amqp.Dial(url) → 包装为 *Conn；失败返回 error（启动失败优于带病运行，基线 §12.1）。
	// 下一轮可补充：连接断开自动重连 + NotifyClose 监听 + 拓扑重声明。
	panic("TODO: queue.Dial not implemented")
}

// DeclareTopology 声明全部拓扑（幂等，可重复调用；总纲 §4.3）：
//  1. direct exchange deepwiki.tasks 与 deepwiki.tasks.dlx（durable=true）；
//  2. 主队列 deepwiki.task.jobs：durable=true，args{x-max-length=c.queueMaxLen,
//     x-dead-letter-exchange=deepwiki.tasks.dlx}，绑定路由键 deepwiki.task.jobs；
//  3. 重试队列 deepwiki.task.retry.{5s,30s,5m}：durable=true，args{x-message-ttl=5000/30000/300000,
//     x-dead-letter-exchange=deepwiki.tasks, x-dead-letter-routing-key=deepwiki.task.jobs}（TTL 到期回流主队列）；
//  4. 死信队列 deepwiki.task.dlq：durable=true，绑定 deepwiki.tasks.dlx。
//
// 验收口径：RabbitMQ management（http://localhost:15672）可见 deepwiki.task.jobs 且 x-max-length=100。
func (c *Conn) DeclareTopology(ctx context.Context) error {
	// TODO: 开临时 channel 按上述顺序 ExchangeDeclare/QueueDeclare/QueueBind 后关闭；
	// 全部声明完成后 logger.Info("rabbitmq topology declared", zap.String("exchange", ExchangeTasks), zap.String("queue", QueueJobs))。
	panic("TODO: Conn.DeclareTopology not implemented")
}

// Channel 新建 channel（publisher/consumer 各自持独立 channel，禁止跨 goroutine 共享，amqp091-go 线程安全约束）。
func (c *Conn) Channel() (*amqp.Channel, error) {
	return c.conn.Channel()
}

// QueueMaxLen 主队列 x-max-length 配置值（背压预检阈值，Manager.Submit 用）。
func (c *Conn) QueueMaxLen() int { return c.queueMaxLen }

// Close 优雅关闭连接（硬约束 #10：在 consumer 停止、在途消息 nack 处理完毕后最后调用）。
func (c *Conn) Close() error {
	if c.conn != nil && !c.conn.IsClosed() {
		return c.conn.Close()
	}
	return nil
}
