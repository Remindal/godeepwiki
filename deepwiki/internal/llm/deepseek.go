package llm

import (
	"context"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// deepseekDefaultBaseURL DeepSeek OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const deepseekDefaultBaseURL = "https://api.deepseek.com"

// DeepSeekLLM 复用 openai-go 客户端的 DeepSeek 对话实现骨架（OpenAI 兼容端点）。
type DeepSeekLLM struct {
	cfg     config.LLMConfig
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewDeepSeekLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *DeepSeekLLM {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = deepseekDefaultBaseURL
	}
	return &DeepSeekLLM{
		cfg:     cfg,
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *DeepSeekLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 经 openai-go 调用 DeepSeek /chat/completions，要求同 OpenAILLM.Generate ①~⑥
	//（breaker 包裹、SDK 内置重试、密钥禁打印、usage 兜底、SDK 类型不外泄、失败映射 50201）。
	panic("TODO: DeepSeekLLM.Generate not implemented")
}

func (l *DeepSeekLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: openai-go 流式（stream.Next() 迭代），要求同 OpenAILLM.GenerateStream ①~④。
	panic("TODO: DeepSeekLLM.GenerateStream not implemented")
}

func (l *DeepSeekLLM) ProviderName() string { return "deepseek" }
func (l *DeepSeekLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*DeepSeekLLM)(nil)
