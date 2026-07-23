package llm

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// deepseekDefaultBaseURL DeepSeek OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const deepseekDefaultBaseURL = "https://api.deepseek.com"

// DeepSeekLLM 复用 openai-go 客户端的 DeepSeek 对话实现（OpenAI 兼容端点）。
type DeepSeekLLM struct {
	cfg     config.LLMConfig
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewDeepSeekLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *DeepSeekLLM {
	return &DeepSeekLLM{
		cfg:     cfg,
		client:  newOpenAIClient(cfg, deepseekDefaultBaseURL),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *DeepSeekLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	return generateWithOpenAI(ctx, l.client, l.cfg, l.breaker, l.logger, req)
}

func (l *DeepSeekLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	return streamWithOpenAI(ctx, l.client, l.cfg, l.breaker, l.logger, req)
}

func (l *DeepSeekLLM) Ping(ctx context.Context) error {
	_, err := l.Generate(ctx, model.ChatRequest{Messages: []model.ChatMessage{{Role: "user", Content: "ping"}}})
	return err
}

func (l *DeepSeekLLM) BreakerState() string { return stateString(l.breaker.State()) }

func (l *DeepSeekLLM) ProviderName() string { return "deepseek" }
func (l *DeepSeekLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*DeepSeekLLM)(nil)
