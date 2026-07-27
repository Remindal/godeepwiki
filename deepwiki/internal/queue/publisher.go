package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"deepwiki/internal/observability"
)

// ErrPublishFailed 投递失败（confirm 未被确认 / mandatory 路由失败 / 连接断开）；
// Manager.Submit 捕获后把任务标记 failed（50302 queue_unavailable，总纲 §6）。
var ErrPublishFailed = errors.New("rabbitmq publish not confirmed")

// Publisher 瘦消息投递器（总纲 §4.3）。实现持有独立 channel 并开启 confirm 模式。
type Publisher interface {
	// Publish 投递瘦消息到 ExchangeTasks（routing key = deepwiki.task.jobs）。
	// mandatory=true：无法路由时 broker 返回 basic.return，必须视为失败；
	// publisher confirm：等待 broker 确认，未确认返回 ErrPublishFailed。
	Publish(ctx context.Context, msg TaskMessage) error
	// QueueDepth 背压预检：QueueDeclarePassive 读主队列 Messages 深度
	//（≥ x-max-length 时 Manager.Submit 拒绝投递 → 42902 + Retry-After，硬约束 #6）。
	QueueDepth(ctx context.Context) (int, error)
	// Close 关闭 channel（不断开共享 Conn）。
	Close() error
}

// RetryPublisher 重试/死信投递扩展接口（消费失败路径用；不污染冻结的 Publisher 接口）。
type RetryPublisher interface {
	// PublishRetry 把消息投递到 ExchangeDLX 的指定重试队列（按 attempt 选择 5s/30s/5m）。
	PublishRetry(ctx context.Context, msg TaskMessage, attempt int) error
	// PublishToDLQ 把消息投递到死信队列（审计）。
	PublishToDLQ(ctx context.Context, msg TaskMessage) error
}

var _ RetryPublisher = (*amqpPublisher)(nil)

const publishTimeout = 5 * time.Second

type amqpPublisher struct {
	conn   *Conn
	logger *zap.Logger
	mu     sync.Mutex
	ch     *amqp.Channel
	closed bool
}

func NewPublisher(conn *Conn, logger *zap.Logger) Publisher {
	return &amqpPublisher{conn: conn, logger: logger}
}

func (p *amqpPublisher) ensureChannel() (*amqp.Channel, error) {
	if p.ch != nil && !p.ch.IsClosed() {
		return p.ch, nil
	}
	// 连接可能半死/已断：先确保连接存活（断开自动重拨），再开 confirm channel。
	if err := p.conn.EnsureConnection(context.Background()); err != nil {
		return nil, err
	}
	ch, err := p.conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, err
	}
	p.ch = ch
	return ch, nil
}

// resetLocked 投递失败后重置：关 channel + 强断连接（下次投递自动重拨重建）。
func (p *amqpPublisher) resetLocked() {
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	p.conn.ForceClose()
}

func (p *amqpPublisher) Publish(ctx context.Context, msg TaskMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal task message: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrPublishFailed
	}
	ch, err := p.ensureChannel()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPublishFailed, err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))

	if err := ch.PublishWithContext(ctx, ExchangeTasks, QueueJobs, true, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Headers:      injectTrace(ctx, nil), // W3C traceparent 跨进程传递（总纲 R16）
	}); err != nil {
		return fmt.Errorf("%w: publish: %v", ErrPublishFailed, err)
	}

	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	select {
	case conf, ok := <-confirms:
		if !ok {
			observability.IncRabbitMQPublishConfirm("fail")
			p.resetLocked()
			return ErrPublishFailed
		}
		if conf.Ack {
			observability.IncRabbitMQPublishConfirm("ok")
			return nil
		}
		observability.IncRabbitMQPublishConfirm("fail")
		p.resetLocked()
		return ErrPublishFailed
	case ret := <-returns:
		p.logger.Warn("rabbitmq message returned", zap.Uint16("replyCode", ret.ReplyCode), zap.String("replyText", ret.ReplyText))
		observability.IncRabbitMQPublishConfirm("fail")
		return ErrPublishFailed
	case <-ctx.Done():
		// confirm 超时：典型半死连接，重置后下次投递自动重拨（用户不再吃到持续的 50302）。
		observability.IncRabbitMQPublishConfirm("fail")
		p.resetLocked()
		return ErrPublishFailed
	}
}

func (p *amqpPublisher) PublishRetry(ctx context.Context, msg TaskMessage, attempt int) error {
	if attempt < 0 || attempt >= len(retryQueues) {
		return p.PublishToDLQ(ctx, msg)
	}
	return p.publishTo(ctx, ExchangeDLX, retryQueues[attempt].name, msg, amqp.Table{retryHeader: int32(attempt + 1)})
}

func (p *amqpPublisher) PublishToDLQ(ctx context.Context, msg TaskMessage) error {
	return p.publishTo(ctx, ExchangeDLX, QueueDLQ, msg, nil)
}

func (p *amqpPublisher) publishTo(ctx context.Context, exchange, routingKey string, msg TaskMessage, headers amqp.Table) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal task message: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrPublishFailed
	}
	ch, err := p.ensureChannel()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPublishFailed, err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Headers:      injectTrace(ctx, headers),
	}); err != nil {
		return fmt.Errorf("%w: publish: %v", ErrPublishFailed, err)
	}

	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	select {
	case conf, ok := <-confirms:
		if !ok || !conf.Ack {
			observability.IncRabbitMQPublishConfirm("fail")
			p.resetLocked()
			return ErrPublishFailed
		}
		observability.IncRabbitMQPublishConfirm("ok")
		return nil
	case <-ctx.Done():
		observability.IncRabbitMQPublishConfirm("fail")
		p.resetLocked()
		return ErrPublishFailed
	}
}

func (p *amqpPublisher) QueueDepth(ctx context.Context) (int, error) {
	// QueueDeclarePassive 是同步 AMQP RPC 且不接受 ctx；半死连接上会永久阻塞
	// （曾导致 health/Submit 挂死）。放独立 goroutine 执行并加 3s 超时兜底。
	type depthResult struct {
		n   int
		err error
	}
	resultCh := make(chan depthResult, 1)
	go func() {
		ch, err := p.conn.Channel()
		if err != nil {
			resultCh <- depthResult{0, err}
			return
		}
		defer ch.Close()
		q, err := ch.QueueDeclarePassive(QueueJobs, true, false, false, false, nil)
		if err != nil {
			resultCh <- depthResult{0, err}
			return
		}
		resultCh <- depthResult{q.Messages, nil}
	}()

	select {
	case r := <-resultCh:
		if r.err != nil {
			var amqpErr *amqp.Error
			if errors.As(r.err, &amqpErr) && amqpErr.Code == 404 {
				// 队列未声明：触发拓扑重声明并按 0 处理，避免健康检查因瞬时缺失而 500。
				if topoErr := p.conn.DeclareTopology(ctx); topoErr != nil {
					p.logger.Warn("rabbitmq topology redeclare failed", zap.Error(topoErr))
				}
				return 0, nil
			}
			return 0, r.err
		}
		return r.n, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(3 * time.Second):
		return 0, fmt.Errorf("rabbitmq queue depth timeout")
	}
}

func (p *amqpPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.ch != nil {
		return p.ch.Close()
	}
	return nil
}
