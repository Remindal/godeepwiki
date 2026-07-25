package task

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// amqpHeaderCarrier 把 AMQP message headers 适配为 OTel TextMapCarrier（只读提取）。
type amqpHeaderCarrier amqp.Table

func (c amqpHeaderCarrier) Get(key string) string {
	if v, ok := amqp.Table(c)[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c amqpHeaderCarrier) Set(key, value string) { /* 只读，跨进程注入由发布方负责 */ }

func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// startDispatchSpan 从 AMQP headers 提取上游 span context 并开启 task.dispatch span。
// OTel 未启用时全局 TracerProvider 为 noop，本函数零成本（总纲 R16）。
func startDispatchSpan(ctx context.Context, taskID string, headers amqp.Table) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, amqpHeaderCarrier(headers))
	tracer := otel.Tracer("deepwiki/task")
	ctx, span := tracer.Start(ctx, "task.dispatch",
		trace.WithAttributes(attribute.String("task_id", taskID)),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	return ctx, span
}

// injectTraceHeaders 发布侧把当前 span context 注入 AMQP headers（Publisher 扩展用）。
func injectTraceHeaders(ctx context.Context, headers propagation.MapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, headers)
}
