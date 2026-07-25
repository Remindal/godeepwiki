package task

import (
	"context"
	"encoding/json"
	"runtime/debug"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/queue"
)

// StartDLQConsumer 启动死信队列消费者（main 以 goroutine 启动，总纲 §4.3）。
// 语义：DLQ 是重试耗尽消息的最终归宿——消费时记录 WARN（task_id、原始队列、重试次数、失败原因），
// 幂等标记 task failed（50003），ack 后不再重试。
func (m *Manager) StartDLQConsumer(ctx context.Context, conn *queue.Conn) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("dlq consumer panic recovered",
					zap.Any("panic", r), zap.String("stack", string(debug.Stack())))
			}
		}()

		ch, err := conn.Channel()
		if err != nil {
			m.logger.Error("dlq consumer channel failed", zap.Error(err))
			return
		}
		deliveries, err := ch.Consume(queue.QueueDLQ, "deepwiki-dlq-consumer", false, false, false, false, nil)
		if err != nil {
			m.logger.Error("dlq consume failed", zap.Error(err))
			return
		}
		m.logger.Info("dlq consumer started", zap.String("queue", queue.QueueDLQ))

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				m.handleDLQ(ctx, d)
			}
		}
	}()
}

func (m *Manager) handleDLQ(ctx context.Context, d amqp.Delivery) {
	var msg queue.TaskMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		m.logger.Error("dlq message unmarshal failed", zap.Error(err), zap.ByteString("body", d.Body))
		_ = d.Ack(false)
		return
	}

	originQueue, deathReason := firstDeath(d.Headers)
	m.logger.Warn("dlq message consumed",
		zap.String("task_id", msg.TaskID),
		zap.String("origin_queue", originQueue),
		zap.Int("retry_count", retryCount(d)),
		zap.String("death_reason", deathReason),
	)

	// 幂等落 failed（worker 重试耗尽时通常已落过；已终态则忽略错误）。
	if ext, ok := m.store.(storeExt); ok {
		_ = ext.FailTask(ctx, msg.TaskID, &model.TaskError{
			Code:    50003,
			Message: "task retry exhausted",
			Stage:   deathReason,
		})
	}
	// 死信不再重试：直接 ack。
	_ = d.Ack(false)
}

// firstDeath 从 x-death header 提取首次死信的原始队列与原因（rejected|expired|maxlen|delivery_limit）。
func firstDeath(h amqp.Table) (queueName, reason string) {
	xd, ok := h["x-death"].([]interface{})
	if !ok || len(xd) == 0 {
		return "", ""
	}
	first, ok := xd[0].(amqp.Table)
	if !ok {
		return "", ""
	}
	queueName, _ = first["queue"].(string)
	reason, _ = first["reason"].(string)
	return queueName, reason
}
