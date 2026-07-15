package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// voyageDefaultBaseURL VoyageAI OpenAI 兼容 embeddings 端点（变更总纲 §4.7，逐字）。
const voyageDefaultBaseURL = "https://api.voyageai.com/v1"

// voyageDims 已知模型维度表；下一轮按官方文档补全。
var voyageDims = map[string]int{
	"voyage-code-3": 1024,
	"voyage-3":      1024,
}

// VoyageEmbedder 复用 openai-go 客户端的 VoyageAI 向量实现骨架（OpenAI 兼容端点，embedding only）。
type VoyageEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewVoyageEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *VoyageEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = voyageDefaultBaseURL
	}
	return &VoyageEmbedder{
		cfg:     cfg,
		dims:    voyageDims[cfg.Model],
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *VoyageEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 经 openai-go 调用 VoyageAI /v1/embeddings，要求同 OpenAIEmbedder.Embed ①~⑥；
	// dims 未知时按首个返回向量长度探测（硬约束 #14）。
	panic("TODO: VoyageEmbedder.Embed not implemented")
}

func (e *VoyageEmbedder) Dimensions() int      { return e.dims }
func (e *VoyageEmbedder) ProviderName() string { return "voyage" }
func (e *VoyageEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*VoyageEmbedder)(nil)
