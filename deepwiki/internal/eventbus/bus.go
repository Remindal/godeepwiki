// Package eventbus 统一事件总线：任务与系统事件的发布订阅，扇出到 SSE / WebSocket / Logger / Metrics（基线 §2.1）。
// 实现为 Redis Streams + Pub/Sub（总纲 §4.4）：每任务一条事件日志流，跨节点扇出到各 API 节点的本地连接。
package eventbus

import (
	"context"

	"deepwiki/internal/model"
)

// EventBus 事件总线抽象（基线 §7，冻结签名）。
type EventBus interface {
	Publish(ctx context.Context, ev model.Event) error
	// Subscribe 返回事件 channel 与取消订阅函数；channel 容量有界（默认 256），
	// 消费者过慢时丢最旧事件并记 WARN（背压保护，禁止阻塞发布者）。
	Subscribe(filter model.EventFilter) (<-chan model.Event, func() /*取消订阅*/)
}

// Replayer 断线补发（冻结语义平移：SSE Last-Event-ID / WS resume_from → XRANGE 回放）；
// 事件 payload 字段冻结（总纲 §2.5），回放仅改变传输来源（Redis Streams），不改变事件结构。
type Replayer interface {
	// ReplaySince 返回 seq > lastSeq 且匹配 filter 的事件；
	// lastSeq 过旧（对应流已被 XTRIM 截断）无法补发时返回 ok=false
	//（调用方推 event: gap，提示回退 GET /api/v1/tasks 全量同步，§6.4）。
	ReplaySince(ctx context.Context, lastSeq uint64, filter model.EventFilter) (events []model.Event, ok bool)
}
