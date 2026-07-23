package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// StartFanout 启动跨节点扇出循环（main 以 goroutine 启动；API 节点把任意 worker 节点产生的事件
// 扇入本节点的 SSE/WS 连接，总纲 §4.4）。
// 流程：SUBSCRIBE events:fanout → 收到 task_id → 检查本节点是否有订阅该任务的 subscriber
// → 有则 XRANGE events:task:<task_id> <lastSeq> + 取增量 → 按 filter 匹配投递到 subscriber.ch。
func (b *RedisStreamsBus) StartFanout(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("eventbus fanout panic recovered",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())),
			)
		}
	}()

	pubsub := b.rdb.Subscribe(ctx, fanoutChannel)
	defer pubsub.Close()
	ch := pubsub.Channel()
	b.logger.Info("eventbus fanout started", zap.String("channel", fanoutChannel))

	for {
		select {
		case <-ctx.Done():
			b.logger.Info("eventbus fanout stopped")
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg.Payload == "" {
				continue
			}
			b.dispatchFanout(ctx, msg.Payload)
		}
	}
}

// dispatchFanout 把 events:task:<taskID> 中 seq > subscriber.lastSeq 的事件推送给本节点订阅者。
func (b *RedisStreamsBus) dispatchFanout(ctx context.Context, taskID string) {
	b.mu.Lock()
	subs := make([]*subscriber, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()
	if len(subs) == 0 {
		return
	}

	stream := streamNameForTask(taskID)
	for _, s := range subs {
		last := s.lastSeq.Load()
		start := fmt.Sprintf("%d-0", last)
		msgs, err := b.rdb.XRangeN(ctx, stream, start, "+", streamMaxLen).Result()
		if err != nil {
			b.logger.Warn("eventbus fanout xrange failed",
				zap.String("stream", stream),
				zap.Int("subscriber_id", s.id),
				zap.Error(err),
			)
			continue
		}

		var maxSeq uint64
		for _, m := range msgs {
			seq, _ := parseStreamSeq(m.ID)
			if seq <= last {
				continue
			}
			if seq > maxSeq {
				maxSeq = seq
			}
			raw, _ := m.Values["data"].(string)
			var ev model.Event
			if err := json.Unmarshal([]byte(raw), &ev); err != nil {
				b.logger.Warn("eventbus fanout unmarshal failed", zap.Error(err))
				continue
			}
			if !matchFilter(ev, s.filter) {
				continue
			}
			b.deliver(s, ev)
		}
		if maxSeq > 0 {
			s.lastSeq.Store(maxSeq)
		}
	}
}

// deliver 向订阅者通道投递；通道满时丢最旧事件并记 WARN（背压保护，禁止阻塞扇出 goroutine）。
func (b *RedisStreamsBus) deliver(s *subscriber, ev model.Event) {
	select {
	case s.ch <- ev:
	default:
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- ev:
		default:
		}
		b.logger.Warn("eventbus subscriber slow, dropped oldest event",
			zap.Int("subscriber_id", s.id),
			zap.Uint64("seq", ev.Seq),
		)
	}
}
