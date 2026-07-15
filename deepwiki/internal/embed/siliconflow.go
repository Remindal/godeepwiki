package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// siliconflowDefaultBaseURL SiliconFlow OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const siliconflowDefaultBaseURL = "https://api.siliconflow.cn/v1"

// siliconflowDims 已知模型维度表；下一轮按官方文档补全。
var siliconflowDims = map[string]int{
	"BAAI/bge-large-zh-v1.5": 1024,
	"BAAI/bge-m3":            1024,
}

// SiliconFlowEmbedder 复用 openai-go 客户端的 SiliconFlow 向量实现骨架（OpenAI 兼容端点）。
type SiliconFlowEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewSiliconFlowEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *SiliconFlowEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = siliconflowDefaultBaseURL
	}
	return &SiliconFlowEmbedder{
		cfg:     cfg,
		dims:    siliconflowDims[cfg.Model],
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *SiliconFlowEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 经 openai-go 调用 SiliconFlow /v1/embeddings，要求同 OpenAIEmbedder.Embed ①~⑥；
	// dims 未知时按首个返回向量长度探测（硬约束 #14）。
	panic("TODO: SiliconFlowEmbedder.Embed not implemented")
}

func (e *SiliconFlowEmbedder) Dimensions() int      { return e.dims }
func (e *SiliconFlowEmbedder) ProviderName() string { return "siliconflow" }
func (e *SiliconFlowEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*SiliconFlowEmbedder)(nil)
