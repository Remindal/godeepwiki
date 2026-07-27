// Package queue RabbitMQ 任务队列：连接管理、拓扑声明、瘦消息投递与消费（总纲 §4.3）。
// 设计要点：RabbitMQ 只承担「任务投递与执行跨进程/跨节点」的传输职责，
// 任务状态唯一来源 = Postgres tasks 表（硬约束 #3）；消息为瘦消息（硬约束 #16）。
package queue

import (
	"context"
	"fmt"
	"sync"

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

	// retryHeader 记录消息已历经的重试次数（消费失败重投时 +1）。
	retryHeader = "x-retry-count"
)

var retryQueues = []struct {
	name string
	ttl  int64
}{
	{QueueRetry5s, 5000},
	{QueueRetry30s, 30000},
	{QueueRetry5m, 300000},
}

// TaskMessage 瘦消息协议（硬约束 #16：body ≤ 4KB，只携带 task_id+type；
// 禁止把任务状态/进度/请求大对象塞进消息——状态唯一来源 = Postgres tasks 表，硬约束 #3）。
type TaskMessage struct {
	TaskID string `json:"task_id"` // tsk_ + ULID(26)
	Type   string `json:"type"`    // ingest|refresh|wiki
}

// Conn RabbitMQ 连接封装（拓扑声明、通道工厂、优雅关闭）。
type Conn struct {
	mu          sync.Mutex
	conn        *amqp.Connection
	url         string
	logger      *zap.Logger
	queueMaxLen int // x-max-length = worker.queue_size（默认 100）
}

// Dial 建立 RabbitMQ 连接（url 仅由环境变量 DEEPWIKI_RABBITMQ_URL 注入，禁止 yaml 明文，硬约束 #2）。
func Dial(ctx context.Context, url string, queueMaxLen int, logger *zap.Logger) (*Conn, error) {
	if url == "" {
		return nil, fmt.Errorf("rabbitmq url is empty")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	return &Conn{conn: conn, url: url, queueMaxLen: queueMaxLen, logger: logger}, nil
}

// EnsureConnection 断线重连：连接已关闭时按原 url 重拨（broker 侧持久拓扑无需重声明）。
// consumer 监督循环在消费 channel 断开时调用（总纲 §4.3 重试语义）。
func (c *Conn) EnsureConnection(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.conn.IsClosed() {
		return nil
	}
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("rabbitmq redial: %w", err)
	}
	c.conn = conn
	c.logger.Warn("rabbitmq reconnected")
	return nil
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
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq topology channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(ExchangeTasks, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq exchange declare %s: %w", ExchangeTasks, err)
	}
	if err := ch.ExchangeDeclare(ExchangeDLX, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq exchange declare %s: %w", ExchangeDLX, err)
	}

	jobArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLX,
		"x-max-length":           int64(c.queueMaxLen),
		"x-overflow":             "reject-publish",
	}
	if _, err := ch.QueueDeclare(QueueJobs, true, false, false, false, jobArgs); err != nil {
		return fmt.Errorf("rabbitmq queue declare %s: %w", QueueJobs, err)
	}
	if err := ch.QueueBind(QueueJobs, QueueJobs, ExchangeTasks, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue bind %s: %w", QueueJobs, err)
	}

	for _, rq := range retryQueues {
		retryArgs := amqp.Table{
			"x-message-ttl":             rq.ttl,
			"x-dead-letter-exchange":    ExchangeTasks,
			"x-dead-letter-routing-key": QueueJobs,
			"x-max-length":              int64(c.queueMaxLen),
		}
		if _, err := ch.QueueDeclare(rq.name, true, false, false, false, retryArgs); err != nil {
			return fmt.Errorf("rabbitmq queue declare %s: %w", rq.name, err)
		}
		if err := ch.QueueBind(rq.name, rq.name, ExchangeDLX, false, nil); err != nil {
			return fmt.Errorf("rabbitmq queue bind %s: %w", rq.name, err)
		}
	}

	if _, err := ch.QueueDeclare(QueueDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue declare %s: %w", QueueDLQ, err)
	}
	// DLQ 既收显式死信路由键，也兜底所有从主队列 nack requeue=false 过来的消息（其 routing key 仍为 QueueJobs）。
	if err := ch.QueueBind(QueueDLQ, QueueDLQ, ExchangeDLX, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue bind %s: %w", QueueDLQ, err)
	}
	if err := ch.QueueBind(QueueDLQ, QueueJobs, ExchangeDLX, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue bind %s (catch-all): %w", QueueDLQ, err)
	}

	c.logger.Info("rabbitmq topology declared",
		zap.String("exchange", ExchangeTasks),
		zap.String("queue", QueueJobs),
		zap.Int("queue_max_len", c.queueMaxLen),
	)
	return nil
}

// ForceClose 强制断开当前连接（半死连接上 confirm 超时后调用，下次使用经 EnsureConnection 重拨）。
func (c *Conn) ForceClose() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.conn.IsClosed() {
		_ = c.conn.Close()
	}
}

// Channel 新建 channel（publisher/consumer 各自持独立 channel，禁止跨 goroutine 共享，amqp091-go 线程安全约束）。
func (c *Conn) Channel() (*amqp.Channel, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq connection closed")
	}
	return conn.Channel()
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
