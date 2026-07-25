package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
)

// HeaderCarrier 把 AMQP message headers 适配为 OTel TextMapCarrier（读：非 string 值跳过；写：string 值注入）。
type HeaderCarrier amqp.Table

func (c HeaderCarrier) Get(key string) string {
	if v, ok := amqp.Table(c)[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c HeaderCarrier) Set(key, value string) { amqp.Table(c)[key] = value }

func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// injectTrace 把当前 span context 注入 AMQP headers（OTel 未启用时 noop，零成本）。
// headers 为 nil 时新建；W3C traceparent 跨进程传递（总纲 R16）。
func injectTrace(ctx context.Context, headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	otel.GetTextMapPropagator().Inject(ctx, HeaderCarrier(headers))
	return headers
}
