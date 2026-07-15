package eventbus

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// Redis 键名与频道（总纲 §4.4，逐字一致，禁止改名）。
const (
	// streamKeyPrefix 每任务事件日志流：events:task:<task_id>（XADD 追加 + XTRIM MAXLEN ~ 1000 截断）。
	streamKeyPrefix = "events:task:"
	// fanoutChannel 跨节点扇出频道：events:fanout（PUBLISH <task_id>，见 redis_fanout.go）。
	fanoutChannel = "events:fanout"
	// streamMaxLen 每任务事件日志近似上限（XTRIM MAXLEN ~ 1000，等价原 1000 条回放窗口语义）。
	streamMaxLen = 1000
	// subChanCap 订阅者 channel 容量（背压保护：满时丢最旧 + WARN）。
	subChanCap = 256
)

type subscriber struct {
	filter model.EventFilter
	ch     chan model.Event
	lastSeq uint64 // 本订阅已扇出到的最大 seq（fanout 增量 XRANGE 起点）
}

// RedisStreamsBus Redis Streams 事件总线实现（总纲 §4.4）。
type RedisStreamsBus struct {
	rdb    redis.UniversalClient
	seq    atomic.Uint64
	mu     sync.Mutex
	subs   map[int]*subscriber
	nextID int
	logger *zap.Logger
}

func NewRedisStreamsBus(rdb redis.UniversalClient, logger *zap.Logger) *RedisStreamsBus {
	return &RedisStreamsBus{
		rdb:    rdb,
		subs:   make(map[int]*subscriber),
		logger: logger,
	}
}

var (
	_ EventBus = (*RedisStreamsBus)(nil)
	_ Replayer = (*RedisStreamsBus)(nil)
)

func (b *RedisStreamsBus) Publish(ctx context.Context, ev model.Event) error {
	// TODO: 实现发布，要求（总纲 §4.4，事件 payload 冻结）：
	// ① b.seq.Add(1) 赋给 ev.Seq、补 ev.Timestamp（UTC，硬约束 #13）；
	// ② XADD events:task:<task_id> * seq <n> type <t> data <json>（结构化 JSON 序列化，禁止拼字符串）；
	// ③ XTRIM events:task:<task_id> MAXLEN ~ 1000（近似截断，保留最近 1000 条供回放）；
	// ④ PUBLISH events:fanout <task_id>（跨节点扇出通知，body 仅 task_id）；
	// ⑤ 本节点本地扇出：命中 filter 的 subscriber 直接投递（本进程内事件免一次网络往返）；
	// ⑥ 指标 deepwiki_redis_op_duration_seconds{op="xadd"} 计时（总纲 §4.8）。
	b.seq.Add(1)
	return nil
}

func (b *RedisStreamsBus) Subscribe(filter model.EventFilter) (<-chan model.Event, func()) {
	// TODO: 注册 subscriber（ch 容量 subChanCap=256），返回 ch 与取消订阅函数
	//（从 map 删除并关闭 ch，防 goroutine 泄漏，硬约束 #4）。
	// 骨架阶段先返回空 channel 占位（events/ws handler 同样为未实现占位，不会订阅）。
	ch := make(chan model.Event, subChanCap)
	return ch, func() {}
}

func (b *RedisStreamsBus) ReplaySince(ctx context.Context, lastSeq uint64, filter model.EventFilter) ([]model.Event, bool) {
	// TODO: 实现回放，要求（总纲 §4.4）：
	// ① filter 必须能定位到 task_id（SSE/WS 连接均绑定任务上下文）；XRANGE events:task:<task_id> <last> + 全量取出；
	// ② 反序列化后按 filter.Types / filter.RepoID 过滤，按 seq 升序返回；
	// ③ lastSeq != 0 且流中最旧 seq > lastSeq+1（流已被 XTRIM 截断）→ 返回 ok=false（调用方推 event: gap，
	//    提示回退 GET /api/v1/tasks 全量同步，§6.4 冻结语义）。
	panic("TODO: RedisStreamsBus.ReplaySince not implemented")
}

// Close 优雅退出时调用（硬约束 #10：关 EventBus 先于关基础设施连接）。
func (b *RedisStreamsBus) Close() {
	// TODO: 关闭全部 subscriber channel（§4.6 / §10.10 优雅退出顺序）。
}
