package queue

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const testRabbitURL = "amqp://deepwiki:deepwiki@localhost:5672/"

func testConn(t *testing.T) *Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := Dial(ctx, testRabbitURL, 100, zap.NewNop())
	if err != nil {
		t.Skipf("rabbitmq not available: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.DeclareTopology(ctx); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
	return conn
}

func TestQueue_TopologyAndPublishConsume(t *testing.T) {
	ctx := context.Background()
	conn := testConn(t)

	pub := NewPublisher(conn, zap.NewNop())
	defer pub.Close()

	// 清空队列避免历史消息干扰。
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = ch.QueuePurge(QueueJobs, false)
	_ = ch.Close()

	msg := TaskMessage{TaskID: "tsk_01J2X9K7QZ0ABCDEFGHJKMNP", Type: "ingest"}
	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatalf("publish: %v", err)
	}

	depth, err := pub.QueueDepth(ctx)
	if err != nil {
		t.Fatalf("queue depth: %v", err)
	}
	if depth != 1 {
		t.Fatalf("queue depth want 1 got %d", depth)
	}

	consumer := NewConsumer(conn, 2, zap.NewNop())
	deliveries, err := consumer.Deliveries(ctx)
	if err != nil {
		t.Fatalf("deliveries: %v", err)
	}

	select {
	case d := <-deliveries:
		if string(d.Body) == "" {
			t.Fatal("empty body")
		}
		if err := d.Ack(false); err != nil {
			t.Fatalf("ack: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	_ = consumer.Stop(ctx)
}

func TestQueue_QueueExists(t *testing.T) {
	conn := testConn(t)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

	if _, err := ch.QueueDeclarePassive(QueueJobs, true, false, false, false, nil); err != nil {
		t.Fatalf("passive declare: %v", err)
	}
}

func TestQueue_RetryPublisher(t *testing.T) {
	ctx := context.Background()
	conn := testConn(t)

	pub := NewPublisher(conn, zap.NewNop())
	defer pub.Close()
	rp := pub.(RetryPublisher)

	msg := TaskMessage{TaskID: "tsk_retry", Type: "ingest"}
	if err := rp.PublishRetry(ctx, msg, 0); err != nil {
		t.Fatalf("publish retry: %v", err)
	}
	if err := rp.PublishToDLQ(ctx, msg); err != nil {
		t.Fatalf("publish dlq: %v", err)
	}

	ch, _ := conn.Channel()
	defer ch.Close()
	q5s, _ := ch.QueueDeclarePassive(QueueRetry5s, true, false, false, false, nil)
	qdlq, _ := ch.QueueDeclarePassive(QueueDLQ, true, false, false, false, nil)
	if q5s.Messages != 1 {
		t.Fatalf("retry 5s messages want 1 got %d", q5s.Messages)
	}
	if qdlq.Messages != 1 {
		t.Fatalf("dlq messages want 1 got %d", qdlq.Messages)
	}
	_, _ = ch.QueuePurge(QueueRetry5s, false)
	_, _ = ch.QueuePurge(QueueDLQ, false)
}

func init() {
	// 避免未使用的 amqp 引用在某些 Go 版本报 compile error（实际测试使用 amqp.Delivery 等）。
	_ = amqp.Persistent
}
