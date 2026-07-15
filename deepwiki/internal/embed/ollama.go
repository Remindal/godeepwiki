package embed

import (
	"context"
	"net/http"
	"net/url"

	"deepwiki/internal/config"

	"github.com/ollama/ollama/api"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// ollamaDefaultBaseURL 本地 Ollama 默认地址（变更总纲 §4.7：base_url 指向本地）。
const ollamaDefaultBaseURL = "http://127.0.0.1:11434"

// OllamaEmbedder 基于 ollama api 包的本地向量实现骨架。
type OllamaEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  *api.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewOllamaEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OllamaEmbedder {
	raw := cfg.BaseURL
	if raw == "" {
		raw = ollamaDefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil {
		base, _ = url.Parse(ollamaDefaultBaseURL) // base_url 格式校验在 config 层兜底
	}
	return &OllamaEmbedder{
		cfg:     cfg,
		client:  api.NewClient(base, &http.Client{Timeout: cfg.Timeout}),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 用 ollama api 包 client.Embed(ctx, &api.EmbedRequest{Model: e.cfg.Model, Input: texts})，要求：
	// ① ollama SDK 无内置重试 → 必须外包一层手写指数退避 backoff×2^n + ±20% 抖动，
	//    仅对网络错 / 429 / 5xx 重试（变更总纲 §4.7，硬约束 #7）；
	// ② 整个调用经 e.breaker.Execute 包裹（连续失败 ≥5 → open 60s → half-open 单探测）；
	// ③ dims 用首次成功返回的 len(embeddings[0]) 探测并缓存（硬约束 #14 维度探测用）；
	// ④ SDK 类型（api.EmbedResponse 等）禁止外泄，方法内转换为 [][]float32（硬约束 #17）；
	// ⑤ 重试耗尽 / breaker open 映射 50202 embedding_unavailable。
	panic("TODO: OllamaEmbedder.Embed not implemented")
}

func (e *OllamaEmbedder) Dimensions() int      { return e.dims }
func (e *OllamaEmbedder) ProviderName() string { return "ollama" }
func (e *OllamaEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*OllamaEmbedder)(nil)
