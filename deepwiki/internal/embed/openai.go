package embed

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// openAIDims 已知模型维度表；未知模型 dims=0，运行时按首次返回向量长度探测（基线 §8.3）。
var openAIDims = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
}

// OpenAIEmbedder 基于官方 SDK openai-go 的 OpenAI 向量实现（变更总纲 §4.7）。
type OpenAIEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

// NewOpenAIEmbedder 构造 OpenAI 向量 adapter；breaker 由工厂注入（每 provider 实例一个，硬约束 #7）。
func NewOpenAIEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		cfg:     cfg,
		dims:    openAIDims[cfg.Model],
		client:  newOpenAIClient(cfg, ""),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedWithOpenAI(ctx, e.client, e.cfg.Model, &e.dims, e.cfg.BatchSize, e.breaker, e.logger, texts)
}

// Ping 健康探测：用最小输入试调用一次 Embed（会被 breaker 统计）。
func (e *OpenAIEmbedder) Ping(ctx context.Context) error {
	_, err := e.Embed(ctx, []string{"dimension probe"})
	return err
}

// BreakerState 返回熔断器状态 closed|open|half-open（health 用）。
func (e *OpenAIEmbedder) BreakerState() string { return stateString(e.breaker.State()) }

func (e *OpenAIEmbedder) Dimensions() int      { return e.dims }
func (e *OpenAIEmbedder) ProviderName() string { return "openai" }
func (e *OpenAIEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*OpenAIEmbedder)(nil)
