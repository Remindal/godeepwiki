package task

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"deepwiki/internal/queue"
)

// startDispatchSpan 从 AMQP headers 提取上游 span context 并开启 task.dispatch span。
// OTel 未启用时全局 TracerProvider 为 noop，本函数零成本（总纲 R16）。
func startDispatchSpan(ctx context.Context, taskID string, headers amqp.Table) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, queue.HeaderCarrier(headers))
	tracer := otel.Tracer("deepwiki/task")
	ctx, span := tracer.Start(ctx, "task.dispatch",
		trace.WithAttributes(attribute.String("task_id", taskID)),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	return ctx, span
}
