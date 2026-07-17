package embed

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// voyageDefaultBaseURL VoyageAI OpenAI 兼容 embeddings 端点（变更总纲 §4.7，逐字）。
const voyageDefaultBaseURL = "https://api.voyageai.com/v1"

// voyageDims 已知模型维度表；未知模型 dims=0 时按首次返回向量长度探测。
var voyageDims = map[string]int{
	"voyage-code-3": 1024,
	"voyage-3":      1024,
}

// VoyageEmbedder 复用 openai-go 客户端的 VoyageAI 向量实现（OpenAI 兼容端点，embedding only）。
type VoyageEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewVoyageEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *VoyageEmbedder {
	return &VoyageEmbedder{
		cfg:     cfg,
		dims:    voyageDims[cfg.Model],
		client:  newOpenAIClient(cfg, voyageDefaultBaseURL),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *VoyageEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedWithOpenAI(ctx, e.client, e.cfg.Model, &e.dims, e.cfg.BatchSize, e.breaker, e.logger, texts)
}

func (e *VoyageEmbedder) Ping(ctx context.Context) error {
	_, err := e.Embed(ctx, []string{"dimension probe"})
	return err
}

func (e *VoyageEmbedder) BreakerState() string { return stateString(e.breaker.State()) }

func (e *VoyageEmbedder) Dimensions() int      { return e.dims }
func (e *VoyageEmbedder) ProviderName() string { return "voyage" }
func (e *VoyageEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*VoyageEmbedder)(nil)
