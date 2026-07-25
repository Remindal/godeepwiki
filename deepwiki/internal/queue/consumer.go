package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	reconnectInterval = 5 * time.Second
	reconnectMaxTries = 10
)

// Consumer RabbitMQ 消费端封装（总纲 §4.3）：
// 每 worker 节点 prefetch = worker.pool_size（默认 2，硬约束 #6 并发上限）；
// manual ack——终态落库成功后才 ack；瞬时错误/panic → nack requeue=false 进 DLX 重试链；
// 优雅退出时未完成消息 nack requeue=true 让别的节点接走（硬约束 #10）。
// 断线重连：channel/connection 断开时监督循环自动重建（间隔 5s、最多 10 次，超出 FATAL 退出）。
type Consumer struct {
	conn     *Conn
	prefetch int
	logger   *zap.Logger
	mu       sync.Mutex
	ch       *amqp.Channel
	tag      string
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewConsumer(conn *Conn, prefetch int, logger *zap.Logger) *Consumer {
	if prefetch <= 0 {
		prefetch = 2
	}
	return &Consumer{conn: conn, prefetch: prefetch, logger: logger, stopCh: make(chan struct{})}
}

// Deliveries 启动消费并返回消息通道（autoAck=false，禁止自动确认，硬约束 #18）。
// 内部由监督循环维护：channel 断开自动重连并恢复消费；ctx 取消或 Stop 后关闭返回通道。
func (c *Consumer) Deliveries(ctx context.Context) (<-chan amqp.Delivery, error) {
	out := make(chan amqp.Delivery, c.prefetch*2)
	go c.supervise(ctx, out)
	return out, nil
}

// supervise 消费监督循环：建立消费 → 转发消息 → 断线重连。
func (c *Consumer) supervise(ctx context.Context, out chan<- amqp.Delivery) {
	defer close(out)
	retries := 0
	for {
		deliveries, err := c.startConsume()
		if err != nil {
			retries++
			if retries > reconnectMaxTries {
				c.logger.Fatal("rabbitmq consumer reconnect failed after max retries",
					zap.Int("max_retries", reconnectMaxTries), zap.Error(err))
			}
			c.logger.Error("rabbitmq consume unavailable, retrying",
				zap.Int("attempt", retries), zap.Int("max_retries", reconnectMaxTries), zap.Error(err))
			if !sleepOrDone(ctx, reconnectInterval) {
				return
			}
			continue
		}
		retries = 0

		if c.pump(ctx, out, deliveries) {
			return // ctx 取消或 Stop：正常退出
		}
		c.logger.Error("rabbitmq consumer channel lost, reconnecting")
		if !sleepOrDone(ctx, reconnectInterval) {
			return
		}
	}
}

// startConsume 建立（或重建）消费：先确保连接存活，再开 channel + Qos + Consume。
func (c *Consumer) startConsume() (<-chan amqp.Delivery, error) {
	if err := c.conn.EnsureConnection(context.Background()); err != nil {
		return nil, err
	}
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("consumer channel: %w", err)
	}
	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consumer qos: %w", err)
	}

	tag := fmt.Sprintf("deepwiki-worker-%d", time.Now().UnixNano())
	deliveries, err := ch.Consume(QueueJobs, tag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consumer consume: %w", err)
	}

	c.mu.Lock()
	c.ch = ch
	c.tag = tag
	c.mu.Unlock()
	c.logger.Info("rabbitmq consumer started", zap.String("tag", tag), zap.Int("prefetch", c.prefetch))
	return deliveries, nil
}

// pump 把当前 channel 的消息转发到 out；返回 true 表示应退出监督循环（ctx 取消/Stop）。
func (c *Consumer) pump(ctx context.Context, out chan<- amqp.Delivery, deliveries <-chan amqp.Delivery) bool {
	for {
		select {
		case <-ctx.Done():
			return true
		case <-c.stopCh:
			return true
		case d, ok := <-deliveries:
			if !ok {
				return false // channel 断开：触发重连
			}
			select {
			case out <- d:
			case <-ctx.Done():
				return true
			}
		}
	}
}

// Stop 停拉新消息（优雅退出第一步，硬约束 #10）；在途消息由 Worker Pool 排空或 nack requeue=true。
func (c *Consumer) Stop(ctx context.Context) error {
	c.stopOnce.Do(func() { close(c.stopCh) })

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

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
