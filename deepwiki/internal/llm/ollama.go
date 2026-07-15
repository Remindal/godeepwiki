package llm

import (
	"context"
	"net/http"
	"net/url"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/ollama/ollama/api"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// ollamaDefaultBaseURL 本地 Ollama 默认地址（变更总纲 §4.7：base_url 指向本地）。
const ollamaDefaultBaseURL = "http://127.0.0.1:11434"

// OllamaLLM 基于 ollama api 包的本地对话实现骨架。
type OllamaLLM struct {
	cfg     config.LLMConfig
	client  *api.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewOllamaLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OllamaLLM {
	raw := cfg.BaseURL
	if raw == "" {
		raw = ollamaDefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil {
		base, _ = url.Parse(ollamaDefaultBaseURL) // base_url 格式校验在 config 层兜底
	}
	return &OllamaLLM{
		cfg:     cfg,
		client:  api.NewClient(base, &http.Client{Timeout: cfg.Timeout}),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *OllamaLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 ollama api 包 client.Chat(ctx, &api.ChatRequest{Model: l.cfg.Model, Messages: ..., Stream: &false}, fn)，要求：
	// ① ollama SDK 无内置重试 → 外包一层手写指数退避 backoff×2^n + ±20% 抖动，仅网络错 / 429 / 5xx（硬约束 #7）；
	// ② 整个调用经 l.breaker.Execute 包裹；③ model.ChatMessage → api.Message 转换只允许在本 adapter 内（硬约束 #17）；
	// ④ usage 缺失按 tokens≈ceil(len([]rune(content))/4) 兜底；⑤ 失败映射 50201 llm_unavailable。
	panic("TODO: OllamaLLM.Generate not implemented")
}

func (l *OllamaLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: ollama 流式（ChatRequest.Stream=true，回调内逐条接收 api.ChatResponse）→ model.StreamChunk channel，要求：
	// ① goroutine defer recover() 且传 ctx（硬约束 #4）；② 流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ③ Done 时结束 chunk 携带 FinishReason / Usage（EvalCount 等）；④ SDK 类型不外泄（硬约束 #17）。
	panic("TODO: OllamaLLM.GenerateStream not implemented")
}

func (l *OllamaLLM) ProviderName() string { return "ollama" }
func (l *OllamaLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*OllamaLLM)(nil)
