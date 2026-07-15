// Package queue RabbitMQ 连接封装与拓扑声明。
// 总纲 §4.5：统一 deepwiki.tasks exchange；主队列 deepwiki.task.jobs（x-max-length 默认 10k，
// 可经 queueMaxLen 覆盖）；DLX → retry 队列 → 死信队列 deepwiki.task.dead.letters。
package queue

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	ExchangeTasks = "deepwiki.tasks"
	QueueJobs     = "deepwiki.task.jobs"
	QueueRetry    = "deepwiki.task.retry"
	QueueDead     = "deepwiki.task.dead.letters"
	RetryDelayMS  = 30000
)

// TaskMessage 投递到任务队列的瘦消息。
type TaskMessage struct {
	TaskID   string `json:"task_id"`
	RepoID   string `json:"repo_id"`
	Action   string `json:"action"`
	Priority int    `json:"priority"`
}

// Conn RabbitMQ 连接封装。
type Conn struct {
	conn        *amqp091.Connection
	queueMaxLen int
	logger      *zap.Logger
}

// Dial 建立 RabbitMQ 连接。
func Dial(ctx context.Context, url string, queueMaxLen int, logger *zap.Logger) (*Conn, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	return &Conn{conn: conn, queueMaxLen: queueMaxLen, logger: logger}, nil
}

// Channel 获取新通道。
func (c *Conn) Channel() (*amqp091.Channel, error) {
	return c.conn.Channel()
}

// DeclareTopology 声明 exchange、主队列、DLX、retry 队列、死信队列及绑定关系。
func (c *Conn) DeclareTopology(ctx context.Context) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(ExchangeTasks, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq exchange declare: %w", err)
	}

	jobArgs := amqp091.Table{
		"x-dead-letter-exchange":    ExchangeTasks,
		"x-dead-letter-routing-key": QueueRetry,
		"x-max-length":              int64(c.queueMaxLen),
		"x-overflow":                "reject-publish",
	}
	if _, err := ch.QueueDeclare(QueueJobs, true, false, false, false, jobArgs); err != nil {
		return fmt.Errorf("rabbitmq queue declare jobs: %w", err)
	}
	if err := ch.QueueBind(QueueJobs, QueueJobs, ExchangeTasks, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue bind jobs: %w", err)
	}

	retryArgs := amqp091.Table{
		"x-dead-letter-exchange":    ExchangeTasks,
		"x-dead-letter-routing-key": QueueDead,
		"x-message-ttl":             int64(RetryDelayMS),
		"x-max-length":              int64(c.queueMaxLen),
	}
	if _, err := ch.QueueDeclare(QueueRetry, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("rabbitmq queue declare retry: %w", err)
	}
	if err := ch.QueueBind(QueueRetry, QueueRetry, ExchangeTasks, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue bind retry: %w", err)
	}

	deadArgs := amqp091.Table{"x-max-length": int64(c.queueMaxLen)}
	if _, err := ch.QueueDeclare(QueueDead, true, false, false, false, deadArgs); err != nil {
		return fmt.Errorf("rabbitmq queue declare dead: %w", err)
	}
	if err := ch.QueueBind(QueueDead, QueueDead, ExchangeTasks, false, nil); err != nil {
		return fmt.Errorf("rabbitmq queue bind dead: %w", err)
	}

	c.logger.Info("rabbitmq topology declared",
		zap.String("exchange", ExchangeTasks),
		zap.String("queue", QueueJobs),
		zap.Int("queue_max_len", c.queueMaxLen),
	)
	return nil
}

// QueueMaxLen 返回主队列最大长度。
func (c *Conn) QueueMaxLen() int { return c.queueMaxLen }

// Close 关闭连接。
func (c *Conn) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Publish 发布消息到任务队列。
func (c *Conn) Publish(ctx context.Context, routingKey string, body []byte, headers amqp091.Table) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq publish channel: %w", err)
	}
	defer ch.Close()
	return ch.PublishWithContext(ctx, ExchangeTasks, routingKey, true, false, amqp091.Publishing{
		ContentType:  "application/json",
		Body:         body,
		Headers:      headers,
		DeliveryMode: amqp091.Persistent,
	})
}

// EnsureConsumerTag 辅助函数：若 tag 为空则生成 worker tag。
func EnsureConsumerTag(tag string) string {
	if tag != "" {
		return tag
	}
	return "deepwiki-worker-" + strconv.FormatInt(timeNow().UnixNano(), 10)
}

func timeNow() Time { return timeNowFunc() }

type Time interface{ UnixNano() int64 }

var timeNowFunc = func() Time { return timeNowReal{} }

type timeNowReal struct{}

func (timeNowReal) UnixNano() int64 { return timeNowUnixNano() }

func timeNowUnixNano() int64 { return time.Now().UnixNano() }
