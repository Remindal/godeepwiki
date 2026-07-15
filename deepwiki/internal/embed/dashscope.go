package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// dashscopeDefaultBaseURL 阿里云百炼 DashScope OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const dashscopeDefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

// dashscopeDims 已知模型维度表；下一轮按官方文档补全，未知模型 dims=0 时探测。
var dashscopeDims = map[string]int{
	"text-embedding-v3": 1024,
	"text-embedding-v2": 1536,
}

// DashScopeEmbedder 复用 openai-go 客户端的 DashScope 向量实现骨架（OpenAI 兼容端点）。
type DashScopeEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewDashScopeEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *DashScopeEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = dashscopeDefaultBaseURL
	}
	return &DashScopeEmbedder{
		cfg:     cfg,
		dims:    dashscopeDims[cfg.Model],
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *DashScopeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 经 openai-go 调用百炼兼容端点 embeddings，要求同 OpenAIEmbedder.Embed ①~⑥
	//（breaker 包裹、batch 分批、SDK 内置重试、密钥禁打印、SDK 类型不外泄、失败映射 50202）；
	// dims 未知时取首个返回向量长度回填并缓存（维度一致性探测，硬约束 #14）。
	panic("TODO: DashScopeEmbedder.Embed not implemented")
}

func (e *DashScopeEmbedder) Dimensions() int      { return e.dims }
func (e *DashScopeEmbedder) ProviderName() string { return "dashscope" }
func (e *DashScopeEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*DashScopeEmbedder)(nil)
