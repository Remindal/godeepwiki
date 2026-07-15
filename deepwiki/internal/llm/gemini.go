package llm

import (
	"context"
	"fmt"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

// GeminiLLM 基于 google.golang.org/genai 的 Gemini 对话实现骨架（变更总纲 §4.7）。
type GeminiLLM struct {
	cfg     config.LLMConfig
	client  *genai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewGeminiLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *GeminiLLM {
	// genai.NewClient 需要 ctx；工厂无 ctx 入参，这里用 Background，装配层必须在启动阶段完成构造（启动失败优于带病运行）。
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(fmt.Sprintf("genai client init: %v", err))
	}
	return &GeminiLLM{
		cfg:     cfg,
		client:  client,
		breaker: breaker,
		logger:  logger,
	}
}

func (l *GeminiLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 genai 调用 client.Models.GenerateContent(...)，要求：
	// ① 整个调用经 l.breaker.Execute 包裹（硬约束 #7）；
	// ② model.ChatMessage → genai.Content 转换只允许发生在本 adapter 内（硬约束 #17）；
	// ③ 密钥仅经 ClientConfig.APIKey 注入，禁止打印（硬约束 #2）；usage 缺失按估算兜底并记日志；
	// ④ 失败映射 50201 llm_unavailable。
	panic("TODO: GeminiLLM.Generate not implemented")
}

func (l *GeminiLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: genai 流式 GenerateContentStream（迭代器）→ model.StreamChunk channel，要求：
	// ① goroutine defer recover() 且传 ctx（硬约束 #4）；② 流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ③ 结束 chunk 携带 FinishReason / Usage；④ SDK 类型不外泄（硬约束 #17）。
	panic("TODO: GeminiLLM.GenerateStream not implemented")
}

func (l *GeminiLLM) ProviderName() string { return "gemini" }
func (l *GeminiLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*GeminiLLM)(nil)
