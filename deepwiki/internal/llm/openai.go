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

// OpenAILLM 基于官方 SDK openai-go 的 OpenAI 对话实现骨架（变更总纲 §4.7）。
type OpenAILLM struct {
	cfg     config.LLMConfig
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

// NewOpenAILLM 构造 OpenAI 对话 adapter；breaker 由工厂注入（每 provider 实例一个，硬约束 #7）。
func NewOpenAILLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OpenAILLM {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &OpenAILLM{
		cfg:     cfg,
		client:  openai.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *OpenAILLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 openai-go 调用 client.Chat.Completions.New(...)，要求：
	// ① 整个调用经 l.breaker.Execute 包裹（硬约束 #7：连续失败 ≥5 → open 60s → half-open 单探测）；
	// ② ctx 超时由调用方统一 context.WithTimeout 包裹；重试优先用 SDK 内置（429/5xx 指数退避，尊重 Retry-After）；
	// ③ API key 仅经 option.WithAPIKey 注入，禁止打印（硬约束 #2）；
	// ④ provider 不返回 usage 时按 tokens≈ceil(len([]rune(content))/4) 估算兜底并记日志（基线 §12.4）；
	// ⑤ SDK 类型在本方法内转换为 model.ChatResponse，禁止外泄（硬约束 #17）；
	// ⑥ 重试耗尽 / 持续 5xx / breaker open 映射 50201 llm_unavailable。
	panic("TODO: OpenAILLM.Generate not implemented")
}

func (l *OpenAILLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: 用 openai-go 流式 API（stream := client.Chat.Completions.NewStreaming(...)；stream.Next() 迭代，
	// SDK 负责 SSE 解析，禁止手写 SSE 解析），要求：
	// ① 返回 channel 由实现方 goroutine 关闭，goroutine 必须 defer recover()（硬约束 #4）；
	// ② ctx 取消即中断流并退出 goroutine（硬约束 #4）；建立调用经 breaker 包裹，流内错误计入 breaker 失败；
	// ③ 结束 chunk 携带 FinishReason / Usage（provider 支持时）；流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ④ SDK 流式类型转换为 model.StreamChunk，禁止外泄（硬约束 #17）。
	panic("TODO: OpenAILLM.GenerateStream not implemented")
}

func (l *OpenAILLM) ProviderName() string { return "openai" }
func (l *OpenAILLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*OpenAILLM)(nil)
