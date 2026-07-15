package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Consumer RabbitMQ 消费端封装（总纲 §4.3）：
// 每 worker 节点 prefetch = worker.pool_size（默认 2，硬约束 #6 并发上限）；
// manual ack——终态落库成功后才 ack；瞬时错误/panic → nack requeue=false 进 DLX 重试链；
// 优雅退出时未完成消息 nack requeue=true 让别的节点接走（硬约束 #10）。
type Consumer struct {
	conn     *Conn
	prefetch int
	logger   *zap.Logger
	// TODO（下一轮）：消费 channel、consumer tag（Channel.Cancel 停拉新消息用）。
}

func NewConsumer(conn *Conn, prefetch int, logger *zap.Logger) *Consumer {
	return &Consumer{conn: conn, prefetch: prefetch, logger: logger}
}

// Deliveries 启动消费并返回消息通道（autoAck=false，禁止自动确认，硬约束 #18）。
func (c *Consumer) Deliveries(ctx context.Context) (<-chan amqp.Delivery, error) {
	// TODO: 实现消费，要求：
	// ① 独立 channel → Qos(prefetch=c.prefetch, 0, false)（prefetch = pool_size，与 Worker Pool 容量一致）；
	// ② Consume(QueueJobs, consumerTag, autoAck=false, exclusive=false, ...) 返回 <-chan amqp.Delivery；
	// ③ ctx 取消或 Stop 调用 → Channel.Cancel(consumerTag) 停拉新消息（不丢弃在途消息）。
	panic("TODO: Consumer.Deliveries not implemented")
}

// Stop 停拉新消息（优雅退出第一步，硬约束 #10）；在途消息由 Worker Pool 排空或 nack requeue=true。
func (c *Consumer) Stop(ctx context.Context) error {
	// TODO: Channel.Cancel(consumerTag)；幂等。
	return nil
}
