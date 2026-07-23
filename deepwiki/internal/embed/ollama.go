package embed

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// ollamaDefaultBaseURL 本地 Ollama 默认地址（变更总纲 §4.7：base_url 指向本地）。
const ollamaDefaultBaseURL = "http://127.0.0.1:11434"

// OllamaEmbedder 基于 ollama api 包的本地向量实现。
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
		base, _ = url.Parse(ollamaDefaultBaseURL)
	}
	return &OllamaEmbedder{
		cfg:     cfg,
		client:  api.NewClient(base, &http.Client{Timeout: cfg.Timeout}),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	result, err := e.breaker.Execute(func() (any, error) {
		return e.embedWithRetry(ctx, texts)
	})
	if err != nil {
		return nil, fmt.Errorf("embedding unavailable: %w", err)
	}
	return result.([][]float32), nil
}

func (e *OllamaEmbedder) embedWithRetry(ctx context.Context, texts []string) ([][]float32, error) {
	maxRetries := e.cfg.Retry.Max
	if maxRetries <= 0 {
		maxRetries = 3
	}
	backoff := e.cfg.Retry.Backoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := e.client.Embed(ctx, &api.EmbedRequest{
			Model: e.cfg.Model,
			Input: texts,
		})
		if err == nil {
			if e.dims == 0 && len(resp.Embeddings) > 0 && len(resp.Embeddings[0]) > 0 {
				e.dims = len(resp.Embeddings[0])
				e.logger.Info("ollama embedding dimensions probed", zap.Int("dims", e.dims))
			}
			return resp.Embeddings, nil
		}
		lastErr = err
		if attempt == maxRetries {
			break
		}
		if !isRetryableError(err) {
			return nil, err
		}
		sleep := backoff * (1 << attempt)
		sleep += time.Duration(rand.Int63n(int64(sleep)/2+1)) - time.Duration(int64(sleep)/4)
		if sleep > 30*time.Second {
			sleep = 30 * time.Second
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func (e *OllamaEmbedder) Ping(ctx context.Context) error {
	_, err := e.Embed(ctx, []string{"dimension probe"})
	return err
}

func (e *OllamaEmbedder) BreakerState() string { return stateString(e.breaker.State()) }

func (e *OllamaEmbedder) Dimensions() int      { return e.dims }
func (e *OllamaEmbedder) ProviderName() string { return "ollama" }
func (e *OllamaEmbedder) ModelName() string    { return e.cfg.Model }

// isRetryableError 仅对网络错 / 429 / 5xx 允许指数退避重试。
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *api.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= 500
	}
	return true
}

var _ Embedder = (*OllamaEmbedder)(nil)
