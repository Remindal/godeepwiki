package llm

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

// New 按配置构造 LLM。provider 取值冻结：openai|gemini|claude|ollama|deepseek。
// SDK 分支按变更总纲 §4.7：openai/deepseek → openai-go（不同 base_url）；claude → anthropic-sdk-go；
// gemini → google.golang.org/genai；ollama → ollama api 包。
func New(cfg config.LLMConfig, logger *zap.Logger) (LLM, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAILLM(cfg, newBreaker("llm-openai"), logger), nil
	case "gemini":
		return NewGeminiLLM(cfg, newBreaker("llm-gemini"), logger), nil
	case "claude":
		return NewClaudeLLM(cfg, newBreaker("llm-claude"), logger), nil
	case "ollama":
		return NewOllamaLLM(cfg, newBreaker("llm-ollama"), logger), nil
	case "deepseek":
		return NewDeepSeekLLM(cfg, newBreaker("llm-deepseek"), logger), nil
	default:
		return nil, fmt.Errorf("unknown llm provider: %s", cfg.Provider)
	}
}
