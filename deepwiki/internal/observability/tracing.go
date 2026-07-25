package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// InitTracer 初始化 OTel TracerProvider（总纲 R16：gin middleware + worker pipeline span +
// pgx/opensearch/rabbitmq 调用 span；OTLP endpoint 空则禁用，零成本）。
// 返回 shutdown 函数（优雅退出时在 flush 日志前调用，强制导出残余 span）。
func InitTracer(ctx context.Context, endpoint, serviceName string, logger *zap.Logger) (shutdown func(context.Context) error, err error) {
	// W3C tracecontext propagator 全局注册（AMQP headers 跨进程 span context 提取/注入用；
	// 未启用追踪时 Extract/Inject 为 noop，零成本）。
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if endpoint == "" {
		logger.Info("otel tracing disabled (observability.otel_endpoint empty)")
		return func(context.Context) error { return nil }, nil
	}
	// TODO: 实现初始化，要求：
	// ① otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure()) 建 exporter；
	// ② sdktrace.NewTracerProvider(WithBatcher(exporter), WithResource(resource.NewWithAttributes(
	//    semconv.SchemaURL, semconv.ServiceName(serviceName))))；
	// ③ otel.SetTracerProvider(tp)；返回 tp.Shutdown；
	// ④ gin 侧用 otelgin 中间件、worker pipeline 手动 span、pgx/opensearch/rabbitmq 调用点包 span（下一轮接入）。
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName))),
	)
	otel.SetTracerProvider(tp)
	logger.Info("otel tracing enabled", zap.String("endpoint", endpoint))
	return tp.Shutdown, nil
}
