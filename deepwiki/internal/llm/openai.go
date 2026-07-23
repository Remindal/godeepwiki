package llm

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// OpenAILLM 基于官方 SDK openai-go 的 OpenAI 对话实现（变更总纲 §4.7）。
type OpenAILLM struct {
	cfg     config.LLMConfig
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

// NewOpenAILLM 构造 OpenAI 对话 adapter；breaker 由工厂注入（每 provider 实例一个，硬约束 #7）。
func NewOpenAILLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OpenAILLM {
	return &OpenAILLM{
		cfg:     cfg,
		client:  newOpenAIClient(cfg, ""),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *OpenAILLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	return generateWithOpenAI(ctx, l.client, l.cfg, l.breaker, l.logger, req)
}

func (l *OpenAILLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	return streamWithOpenAI(ctx, l.client, l.cfg, l.breaker, l.logger, req)
}

// Ping 健康探测：用最小输入试调用一次 Generate（会被 breaker 统计）。
func (l *OpenAILLM) Ping(ctx context.Context) error {
	_, err := l.Generate(ctx, model.ChatRequest{Messages: []model.ChatMessage{{Role: "user", Content: "ping"}}})
	return err
}

// BreakerState 返回熔断器状态 closed|open|half-open（health 用）。
func (l *OpenAILLM) BreakerState() string { return stateString(l.breaker.State()) }

func (l *OpenAILLM) ProviderName() string { return "openai" }
func (l *OpenAILLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*OpenAILLM)(nil)
