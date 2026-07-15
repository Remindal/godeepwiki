package eventbus

import (
	"context"

	"go.uber.org/zap"
)

// StartFanout 启动跨节点扇出循环（main 以 goroutine 启动；API 节点把任意 worker 节点产生的事件
// 扇入本节点的 SSE/WS 连接，总纲 §4.4）。
// 流程：SUBSCRIBE events:fanout → 收到 task_id → 检查本节点是否有订阅该任务的 subscriber
// → 有则 XRANGE events:task:<task_id> <lastSeq> + 取增量 → 按 filter 匹配投递到 subscriber.ch。
func (b *RedisStreamsBus) StartFanout(ctx context.Context) {
	// TODO: 实现扇出循环，要求：
	// ① rdb.Subscribe(ctx, fanoutChannel) 接收 task_id 通知；断线自动重订阅（go-redis PubSub 重连语义）；
	// ② 命中本节点 subscriber 后按各 subscriber.lastSeq 增量 XRANGE，避免重复推送；
	// ③ subscriber.ch 满（256）时丢最旧并记 WARN（背压保护，禁止阻塞扇出 goroutine）；
	// ④ ctx 取消 → 退订并退出 goroutine；全程 defer recover()（硬约束 #4）；
	// ⑤ 指标 deepwiki_redis_op_duration_seconds{op="fanout"} 计时。
	b.logger.Info("eventbus fanout started", zap.String("channel", fanoutChannel))
}
