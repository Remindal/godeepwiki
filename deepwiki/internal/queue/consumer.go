package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	mu       sync.Mutex
	ch       *amqp.Channel
	tag      string
}

func NewConsumer(conn *Conn, prefetch int, logger *zap.Logger) *Consumer {
	if prefetch <= 0 {
		prefetch = 2
	}
	return &Consumer{conn: conn, prefetch: prefetch, logger: logger}
}

// Deliveries 启动消费并返回消息通道（autoAck=false，禁止自动确认，硬约束 #18）。
func (c *Consumer) Deliveries(ctx context.Context) (<-chan amqp.Delivery, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("consumer channel: %w", err)
	}
	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consumer qos: %w", err)
	}

	c.tag = fmt.Sprintf("deepwiki-worker-%d", time.Now().UnixNano())
	deliveries, err := ch.Consume(QueueJobs, c.tag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consumer consume: %w", err)
	}
	c.ch = ch

	// ctx 取消或 Stop 调用时停止拉取新消息；在途消息 channel 保持打开以允许 ack/nack。
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Stop(context.Background())
		}
	}()

	return deliveries, nil
}

// Stop 停拉新消息（优雅退出第一步，硬约束 #10）；在途消息由 Worker Pool 排空或 nack requeue=true。
func (c *Consumer) Stop(ctx context.Context) error {
	c.mu.Lock()
	ch := c.ch
	tag := c.tag
	c.tag = ""
	c.mu.Unlock()

	if ch == nil || tag == "" {
		return nil
	}
	if err := ch.Cancel(tag, false); err != nil {
		return fmt.Errorf("consumer cancel: %w", err)
	}
	return nil
}
