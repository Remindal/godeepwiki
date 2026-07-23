// Package eventbus 基于 Redis Streams 的事件总线。
// 物理载体为每任务一条流 events:task:<task_id>（XTRIM MAXLEN ~ 1000），
// 跨节点扇出经 Pub/Sub 频道 events:fanout（总纲 §4.4）。
package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

const (
	streamPrefix  = "events:task:"
	fanoutChannel = "events:fanout"
	streamMaxLen  = 1000
	subBuffer     = 256
)

// RedisStreamsBus Redis Streams 实现：每任务一条事件日志流 + Pub/Sub 跨节点扇出。
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
	lastSeq atomic.Uint64
}

// NewRedisStreamsBus 创建 Redis Streams 总线；本地 seq 以 UnixMilli 初始化，避免重启后与历史事件撞号。
func NewRedisStreamsBus(rdb redis.UniversalClient, logger *zap.Logger) *RedisStreamsBus {
	b := &RedisStreamsBus{rdb: rdb, subs: make(map[int]*subscriber), logger: logger}
	b.seq.Store(uint64(time.Now().UnixMilli()))
	return b
}

func streamNameForTask(taskID string) string {
	if taskID == "" {
		return streamPrefix + "global"
	}
	return streamPrefix + taskID
}

// Publish 发布事件到 events:task:<task_id> Stream，并写入扇出频道。
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

	stream := streamNameForTask(ev.TaskID)
	id := fmt.Sprintf("%d-0", ev.Seq)
	args := redis.XAddArgs{
		Stream: stream,
		ID:     id,
		Values: map[string]any{
			"seq":  ev.Seq,
			"type": ev.Type,
			"data": string(payload),
		},
	}
	if _, err := b.rdb.XAdd(ctx, &args).Result(); err != nil {
		return fmt.Errorf("eventbus xadd: %w", err)
	}
	if err := b.rdb.XTrimMaxLenApprox(ctx, stream, streamMaxLen, 0).Err(); err != nil {
		b.logger.Warn("eventbus xtrim failed", zap.String("stream", stream), zap.Error(err))
	}
	if ev.TaskID != "" {
		if err := b.rdb.Publish(ctx, fanoutChannel, ev.TaskID).Err(); err != nil {
			b.logger.Warn("eventbus fanout publish failed", zap.Error(err))
		}
	}
	return nil
}

func (b *RedisStreamsBus) nextSeq() uint64 {
	return b.seq.Add(1)
}

// Subscribe 订阅事件总线；返回过滤后的事件通道与取消函数。
// 新订阅者从当前 seq 起订阅，不回放历史；断线回放由 Replayer.ReplaySince 承担。
func (b *RedisStreamsBus) Subscribe(filter model.EventFilter) (<-chan model.Event, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan model.Event, subBuffer)
	sub := &subscriber{id: b.nextID, ch: ch, cancel: cancel, filter: filter}
	sub.lastSeq.Store(b.seq.Load())

	b.mu.Lock()
	sub.id = b.nextID
	b.nextID++
	b.subs[sub.id] = sub
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.remove(sub.id)
	}()
	return ch, func() { cancel() }
}

// ReplaySince 重放 seq > lastSeq 且匹配 filter 的事件；lastSeq 过旧（流已截断）无法补发时返回 ok=false。
func (b *RedisStreamsBus) ReplaySince(ctx context.Context, lastSeq uint64, filter model.EventFilter) ([]model.Event, bool) {
	if filter.TaskID == "" {
		return nil, false
	}
	stream := streamNameForTask(filter.TaskID)
	start := fmt.Sprintf("%d-0", lastSeq)
	msgs, err := b.rdb.XRangeN(ctx, stream, start, "+", streamMaxLen).Result()
	if err != nil {
		b.logger.Warn("eventbus replay xrange", zap.String("stream", stream), zap.Error(err))
		return nil, false
	}

	var events []model.Event
	var firstSeq uint64
	for _, m := range msgs {
		seq, _ := parseStreamSeq(m.ID)
		if seq <= lastSeq {
			continue
		}
		if firstSeq == 0 {
			firstSeq = seq
		}
		raw, _ := m.Values["data"].(string)
		var ev model.Event
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			b.logger.Warn("eventbus replay unmarshal", zap.Error(err))
			continue
		}
		if matchFilter(ev, filter) {
			events = append(events, ev)
		}
	}

	// 若 lastSeq 非 0 且第一条返回事件的 seq 跳过 lastSeq+1，说明中间事件已被 XTRIM 截断。
	gap := lastSeq > 0 && firstSeq > 0 && firstSeq > lastSeq+1
	return events, !gap
}

func matchFilter(ev model.Event, filter model.EventFilter) bool {
	if filter.TaskID != "" && ev.TaskID != filter.TaskID {
		return false
	}
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

// parseStreamSeq 从 Redis Stream ID "seq-0" 解析 seq。
func parseStreamSeq(id string) (uint64, error) {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid stream id %q", id)
	}
	return strconv.ParseUint(parts[0], 10, 64)
}
