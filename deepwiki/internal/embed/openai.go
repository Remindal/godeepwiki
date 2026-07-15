package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// openAIDims 已知模型维度表；未知模型 dims=0，下一轮实现时探测（基线 §8.3）。
var openAIDims = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
}

// OpenAIEmbedder 基于官方 SDK openai-go 的 OpenAI 向量实现骨架（变更总纲 §4.7）。
type OpenAIEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

// NewOpenAIEmbedder 构造 OpenAI 向量 adapter；breaker 由工厂注入（每 provider 实例一个，硬约束 #7）。
func NewOpenAIEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OpenAIEmbedder {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &OpenAIEmbedder{
		cfg:     cfg,
		dims:    openAIDims[cfg.Model],
		client:  openai.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 用 openai-go 调用 client.Embeddings.New(ctx, openai.EmbeddingNewParams{...})，要求：
	// ① 整个调用必须经 e.breaker.Execute 包裹（硬约束 #7：连续失败 ≥5 → open 60s → half-open 单探测，状态反映到 health）；
	// ② 按 cfg.BatchSize 分批；ctx 超时由调用方统一 context.WithTimeout 包裹（硬约束 #7）；
	// ③ 重试优先用 SDK 内置机制（openai-go 默认对 429/5xx 指数退避并尊重 Retry-After），禁止再手写固定间隔重试；
	// ④ API key 仅经 option.WithAPIKey 注入，禁止打印密钥到日志（硬约束 #2）；
	// ⑤ SDK 返回类型必须在本方法内转换为 [][]float32，禁止外泄到签名（硬约束 #17）；
	// ⑥ 重试耗尽 / breaker open 映射哨兵错误，由上层转 50202 embedding_unavailable。
	panic("TODO: OpenAIEmbedder.Embed not implemented")
}

func (e *OpenAIEmbedder) Dimensions() int      { return e.dims }
func (e *OpenAIEmbedder) ProviderName() string { return "openai" }
func (e *OpenAIEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*OpenAIEmbedder)(nil)
