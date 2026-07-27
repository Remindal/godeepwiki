package embed

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// dashscopeDefaultBaseURL 阿里云百炼 DashScope OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const dashscopeDefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

// dashscopeDims 已知模型维度表；未知模型 dims=0 时按首次返回向量长度探测。
var dashscopeDims = map[string]int{
	"text-embedding-v3": 1024,
	"text-embedding-v2": 1536,
}

// DashScopeEmbedder 复用 openai-go 客户端的 DashScope 向量实现（OpenAI 兼容端点）。
type DashScopeEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewDashScopeEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *DashScopeEmbedder {
	return &DashScopeEmbedder{
		cfg:     cfg,
		dims:    dashscopeDims[cfg.Model],
		client:  newOpenAIClient(cfg, dashscopeDefaultBaseURL),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *DashScopeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedWithOpenAI(ctx, e.client, e.cfg.Model, &e.dims, e.cfg.BatchSize, e.breaker, e.logger, texts, 0)
}

func (e *DashScopeEmbedder) Ping(ctx context.Context) error {
	_, err := e.Embed(ctx, []string{"dimension probe"})
	return err
}

func (e *DashScopeEmbedder) BreakerState() string { return stateString(e.breaker.State()) }

func (e *DashScopeEmbedder) Dimensions() int      { return e.dims }
func (e *DashScopeEmbedder) ProviderName() string { return "dashscope" }
func (e *DashScopeEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*DashScopeEmbedder)(nil)
