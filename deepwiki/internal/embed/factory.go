package embed

import (
	"fmt"
	"time"

	"deepwiki/internal/config"

	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// newBreaker 每个 provider 实例一个熔断器（变更总纲 §4.7 / 硬约束 #7）：
// 连续失败 ≥5 → open 60s → half-open 单探测（MaxRequests=1）→ 关闭；状态反映到 health。
func newBreaker(name string) *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 5 },
	})
}

// New 按配置构造 Embedder。provider 取值冻结：openai|dashscope|siliconflow|ollama|voyage。
// SDK 分支按变更总纲 §4.7：openai/dashscope/siliconflow/voyage → openai-go（不同 base_url）；ollama → ollama api 包。
func New(cfg config.EmbeddingConfig, logger *zap.Logger) (Embedder, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIEmbedder(cfg, newBreaker("embed-openai"), logger), nil
	case "dashscope":
		return NewDashScopeEmbedder(cfg, newBreaker("embed-dashscope"), logger), nil
	case "siliconflow":
		return NewSiliconFlowEmbedder(cfg, newBreaker("embed-siliconflow"), logger), nil
	case "ollama":
		return NewOllamaEmbedder(cfg, newBreaker("embed-ollama"), logger), nil
	case "voyage":
		return NewVoyageEmbedder(cfg, newBreaker("embed-voyage"), logger), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}
