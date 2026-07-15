// Package eventbus 基于 Redis Streams 的事件总线。
// 统一事件形态 model.Event（总纲 §4.3），用于领域事件广播、跨节点扇出、任务状态同步。
package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

const (
	streamName    = "deepwiki:events"
	fanoutPrefix  = "deepwiki:events:fanout:"
	fanoutChannel = "events:fanout"
)

// RedisStreamsBus Redis Streams 实现，带本节点顺序序列号与跨节点扇出。
type RedisStreamsBus struct {
	rdb    redis.UniversalClient
	seq    atomic.Uint64
	mu     sync.Mutex
	subs   map[int]*subscriber
	nextID int
	logger *zap.Logger
}

type subscriber struct {
	id      int
	ch      chan model.Event
	cancel  context.CancelFunc
	filter  model.EventFilter
	onEvent func(model.Event)
}

// NewRedisStreamsBus 创建 Redis Streams 总线。
func NewRedisStreamsBus(rdb redis.UniversalClient, logger *zap.Logger) *RedisStreamsBus {
	return &RedisStreamsBus{rdb: rdb, subs: make(map[int]*subscriber), logger: logger}
}

// Publish 发布事件到 deepwiki:events Stream，并顺带写入扇出频道。
func (b *RedisStreamsBus) Publish(ctx context.Context, ev model.Event) error {
	if ev.Seq == 0 {
		ev.Seq = b.nextSeq()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("eventbus publish marshal: %w", err)
	}
	args := redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{"data": string(payload)},
	}
	if _, err := b.rdb.XAdd(ctx, &args).Result(); err != nil {
		return fmt.Errorf("eventbus xadd: %w", err)
	}
	if err := b.rdb.Publish(ctx, fanoutChannel, payload).Err(); err != nil {
		b.logger.Warn("eventbus fanout publish failed", zap.Error(err))
	}
	return nil
}

func (b *RedisStreamsBus) nextSeq() uint64 {
	return b.seq.Add(1)
}

// Subscribe 订阅事件总线；返回过滤后的事件通道与取消函数。
func (b *RedisStreamsBus) Subscribe(filter model.EventFilter) (<-chan model.Event, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan model.Event, 256)
	sub := &subscriber{id: b.nextID, ch: ch, cancel: cancel, filter: filter}
	b.mu.Lock()
	sub.id = b.nextID
	b.nextID++
	b.subs[sub.id] = sub
	b.mu.Unlock()

	// 本地广播分发 goroutine（由 Publish 触发，占位）。
	go func() {
		<-ctx.Done()
		b.remove(sub.id)
	}()
	return ch, func() { cancel() }
}

// ReplaySince 重放 seq > lastSeq 且匹配 filter 的事件；无法补发时返回 ok=false。
func (b *RedisStreamsBus) ReplaySince(ctx context.Context, lastSeq uint64, filter model.EventFilter) ([]model.Event, bool) {
	start := fmt.Sprintf("%d-0", lastSeq)
	var events []model.Event
	for {
		msgs, err := b.rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamName, start},
			Count:   100,
			Block:   100 * time.Millisecond,
		}).Result()
		if err == redis.Nil {
			select {
			case <-ctx.Done():
				return events, false
			default:
				continue
			}
		}
		if err != nil {
			b.logger.Warn("eventbus replay xread", zap.Error(err))
			return events, false
		}
		gotNew := false
		for _, stream := range msgs {
			for _, msg := range stream.Messages {
				start = msg.ID
				raw, _ := msg.Values["data"].(string)
				var ev model.Event
				if err := json.Unmarshal([]byte(raw), &ev); err != nil {
					b.logger.Warn("eventbus replay unmarshal", zap.Error(err))
					continue
				}
				if ev.Seq <= lastSeq {
					continue
				}
				gotNew = true
				if matchFilter(ev, filter) {
					events = append(events, ev)
				}
			}
		}
		if !gotNew {
			break
		}
		select {
		case <-ctx.Done():
			return events, false
		default:
		}
	}
	return events, true
}

func matchFilter(ev model.Event, filter model.EventFilter) bool {
	if filter.RepoID != "" && ev.RepoID != filter.RepoID {
		return false
	}
	if len(filter.Types) > 0 {
		for _, t := range filter.Types {
			if t == ev.Type {
				return true
			}
		}
		return false
	}
	return true
}

func (b *RedisStreamsBus) remove(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subs[id]; ok {
		sub.cancel()
		close(sub.ch)
		delete(b.subs, id)
	}
}

// Close 关闭总线。
func (b *RedisStreamsBus) Close() error {
	b.mu.Lock()
	for _, sub := range b.subs {
		sub.cancel()
		close(sub.ch)
	}
	b.subs = make(map[int]*subscriber)
	b.mu.Unlock()
	return nil
}

// parseSeqID 从 "millis-seq" 解析时间戳。
func parseSeqID(id string) time.Time {
	millis, _ := strconv.ParseInt(id, 10, 64)
	return time.UnixMilli(millis)
}
