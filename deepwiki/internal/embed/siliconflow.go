package embed

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// siliconflowDefaultBaseURL SiliconFlow OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const siliconflowDefaultBaseURL = "https://api.siliconflow.cn/v1"

// siliconflowDims 已知模型维度表；未知模型 dims=0 时按首次返回向量长度探测。
var siliconflowDims = map[string]int{
	"BAAI/bge-large-zh-v1.5": 1024,
	"BAAI/bge-m3":            1024,
}

// SiliconFlowEmbedder 复用 openai-go 客户端的 SiliconFlow 向量实现（OpenAI 兼容端点）。
type SiliconFlowEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewSiliconFlowEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *SiliconFlowEmbedder {
	return &SiliconFlowEmbedder{
		cfg:     cfg,
		dims:    siliconflowDims[cfg.Model],
		client:  newOpenAIClient(cfg, siliconflowDefaultBaseURL),
		breaker: breaker,
		logger:  logger,
	}
}

// siliconflowMaxInputRunes bge 系列输入上限 512 tokens 的保守 rune 上限（CJK≈1 token/字）。
const siliconflowMaxInputRunes = 480

func (e *SiliconFlowEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return embedWithOpenAI(ctx, e.client, e.cfg.Model, &e.dims, e.cfg.BatchSize, e.breaker, e.logger, texts, siliconflowMaxInputRunes)
}

func (e *SiliconFlowEmbedder) Ping(ctx context.Context) error {
	_, err := e.Embed(ctx, []string{"dimension probe"})
	return err
}

func (e *SiliconFlowEmbedder) BreakerState() string { return stateString(e.breaker.State()) }

func (e *SiliconFlowEmbedder) Dimensions() int      { return e.dims }
func (e *SiliconFlowEmbedder) ProviderName() string { return "siliconflow" }
func (e *SiliconFlowEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*SiliconFlowEmbedder)(nil)
