package queue

import (
	"context"
	"errors"

	"go.uber.org/zap"
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

type amqpPublisher struct {
	conn   *Conn
	logger *zap.Logger
	// TODO（下一轮）：confirm channel、NotifyConfirm/NotifyReturn 监听、序列化缓冲。
}

func NewPublisher(conn *Conn, logger *zap.Logger) Publisher {
	return &amqpPublisher{conn: conn, logger: logger}
}

func (p *amqpPublisher) Publish(ctx context.Context, msg TaskMessage) error {
	// TODO: 实现投递，要求（总纲 §4.3，硬约束 #16）：
	// ① json.Marshal(msg) 为 body（≤ 4KB，天然满足；DeliveryMode=Persistent 持久化，ContentType=application/json）；
	// ② channel.PublishWithContext(ctx, ExchangeTasks, QueueJobs, mandatory=true, immediate=false, ...)；
	// ③ 等待 confirm：ack → 指标 deepwiki_rabbitmq_publish_confirms_total{result="ok"}++；
	//    nack/超时/return → result="fail"++ 并返回 ErrPublishFailed（由 Manager 落库 failed/50302）。
	panic("TODO: amqpPublisher.Publish not implemented")
}

func (p *amqpPublisher) QueueDepth(ctx context.Context) (int, error) {
	ch, err := p.conn.Channel()
	if err != nil {
		return 0, err
	}
	defer ch.Close()
	q, err := ch.QueueDeclarePassive(QueueJobs, true, false, false, false, nil)
	if err != nil {
		return 0, err
	}
	return q.Messages, nil
}

func (p *amqpPublisher) Close() error {
	// TODO: 关闭 channel（幂等）。
	return nil
}
