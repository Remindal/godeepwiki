package llm

import (
	"context"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// ClaudeLLM 基于官方 anthropic-sdk-go 的 Claude 对话实现骨架（变更总纲 §4.7）。
type ClaudeLLM struct {
	cfg     config.LLMConfig
	client  anthropic.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewClaudeLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *ClaudeLLM {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &ClaudeLLM{
		cfg:     cfg,
		client:  anthropic.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *ClaudeLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 anthropic-sdk-go 调用 client.Messages.New(...)，要求：
	// ① 整个调用经 l.breaker.Execute 包裹（硬约束 #7）；
	// ② 模型领域角色映射：model.ChatMessage{system|user|assistant} → SDK 消息参数，
	//    system 消息按 SDK 要求拆到 system 参数；转换只允许发生在本 adapter 内（硬约束 #17）；
	// ③ 重试优先 SDK 内置；密钥禁打印（硬约束 #2）；usage 缺失按 tokens≈ceil(len([]rune)/4) 兜底；
	// ④ 失败映射 50201 llm_unavailable。
	panic("TODO: ClaudeLLM.Generate not implemented")
}

func (l *ClaudeLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: anthropic 流式 API（Messages.NewStreaming，事件迭代）→ model.StreamChunk channel，要求：
	// ① goroutine defer recover() 且传 ctx（硬约束 #4）；② 流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ③ 结束 chunk 携带 FinishReason / Usage；④ SDK 事件类型不外泄（硬约束 #17）。
	panic("TODO: ClaudeLLM.GenerateStream not implemented")
}

func (l *ClaudeLLM) ProviderName() string { return "claude" }
func (l *ClaudeLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*ClaudeLLM)(nil)
