package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

func testBus(t *testing.T) (*RedisStreamsBus, *redis.Client) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}
	return NewRedisStreamsBus(rdb, zap.NewNop()), rdb
}

func TestEventBus_PublishAndReplay(t *testing.T) {
	ctx := context.Background()
	bus, rdb := testBus(t)
	defer rdb.Close()
	defer bus.Close()

	taskID := "tsk_eb_" + time.Now().Format("20060102150405")
	stream := streamNameForTask(taskID)
	_ = rdb.Del(ctx, stream)

	for i := 0; i < 3; i++ {
		ev := model.Event{Type: model.EventTypeTaskStateChanged, TaskID: taskID, RepoID: "repo_test"}
		if err := bus.Publish(ctx, ev); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	events, ok := bus.ReplaySince(ctx, 0, model.EventFilter{TaskID: taskID})
	if !ok {
		t.Fatal("replay not ok")
	}
	if len(events) != 3 {
		t.Fatalf("want 3 events got %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Seq <= events[i-1].Seq {
			t.Fatalf("seq not increasing: %+v", events)
		}
	}

	// 从中间回放。
	last := events[1].Seq
	events2, ok := bus.ReplaySince(ctx, last, model.EventFilter{TaskID: taskID})
	if !ok || len(events2) != 1 || events2[0].Seq != events[2].Seq {
		t.Fatalf("replay since middle failed: %+v ok=%v", events2, ok)
	}
}

func TestEventBus_ReplayGap(t *testing.T) {
	ctx := context.Background()
	bus, rdb := testBus(t)
	defer rdb.Close()
	defer bus.Close()

	taskID := "tsk_eb_gap_" + time.Now().Format("20060102150405")
	stream := streamNameForTask(taskID)
	_ = rdb.Del(ctx, stream)

	for i := 0; i < 3; i++ {
		if err := bus.Publish(ctx, model.Event{Type: model.EventTypeTaskStateChanged, TaskID: taskID}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	// 只保留最新 1 条，模拟历史被截断。
	if err := rdb.XTrimMaxLen(ctx, stream, 1).Err(); err != nil {
		t.Fatalf("xtrim: %v", err)
	}

	events, ok := bus.ReplaySince(ctx, 1, model.EventFilter{TaskID: taskID})
	if ok {
		t.Fatalf("expected gap (ok=false), got events=%+v", events)
	}
}

func TestEventBus_SubscribeFanout(t *testing.T) {
	ctx := context.Background()
	bus, rdb := testBus(t)
	defer rdb.Close()
	defer bus.Close()

	taskID := "tsk_eb_fan_" + time.Now().Format("20060102150405")
	stream := streamNameForTask(taskID)
	_ = rdb.Del(ctx, stream)

	fanCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go bus.StartFanout(fanCtx)
	time.Sleep(200 * time.Millisecond) // 等 Pub/Sub 订阅建立

	ch, unsub := bus.Subscribe(model.EventFilter{TaskID: taskID})
	defer unsub()

	ev := model.Event{Type: model.EventTypeTaskStateChanged, TaskID: taskID, RepoID: "repo_test"}
	if err := bus.Publish(ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case got := <-ch:
		if got.TaskID != taskID || got.Type != model.EventTypeTaskStateChanged {
			t.Fatalf("unexpected event: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for fanout event")
	}
}
