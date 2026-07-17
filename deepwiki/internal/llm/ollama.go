package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// ollamaDefaultBaseURL 本地 Ollama 默认地址（变更总纲 §4.7：base_url 指向本地）。
const ollamaDefaultBaseURL = "http://127.0.0.1:11434"

// OllamaLLM 基于 ollama api 包的本地对话实现。
type OllamaLLM struct {
	cfg     config.LLMConfig
	client  *api.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewOllamaLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OllamaLLM {
	raw := cfg.BaseURL
	if raw == "" {
		raw = ollamaDefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil {
		base, _ = url.Parse(ollamaDefaultBaseURL)
	}
	return &OllamaLLM{
		cfg:     cfg,
		client:  api.NewClient(base, &http.Client{Timeout: cfg.Timeout}),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *OllamaLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	result, err := l.breaker.Execute(func() (any, error) {
		return l.chatWithRetry(ctx, req, false)
	})
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("llm unavailable: %w", err)
	}
	return result.(model.ChatResponse), nil
}

func (l *OllamaLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	ch := make(chan model.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("ollama stream panic", zap.Any("panic", r))
			}
			close(ch)
		}()
		_, err := l.breaker.Execute(func() (any, error) {
			return nil, l.streamWithRetry(ctx, req, ch)
		})
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- model.StreamChunk{Err: fmt.Errorf("llm unavailable: %w", err)}:
			}
		}
	}()
	return ch, nil
}

func (l *OllamaLLM) chatWithRetry(ctx context.Context, req model.ChatRequest, streaming bool) (model.ChatResponse, error) {
	maxRetries := l.cfg.Retry.Max
	if maxRetries <= 0 {
		maxRetries = 3
	}
	backoff := l.cfg.Retry.Backoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var resp model.ChatResponse
		err := l.client.Chat(ctx, l.buildChatRequest(req, false), func(r api.ChatResponse) error {
			if r.Done {
				resp = l.toChatResponse(r)
			}
			return nil
		})
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt == maxRetries || !isRetryableError(err) {
			break
		}
		l.sleepBackoff(ctx, backoff, attempt)
	}
	return model.ChatResponse{}, lastErr
}

func (l *OllamaLLM) streamWithRetry(ctx context.Context, req model.ChatRequest, ch chan<- model.StreamChunk) error {
	maxRetries := l.cfg.Retry.Max
	if maxRetries <= 0 {
		maxRetries = 3
	}
	backoff := l.cfg.Retry.Backoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := l.client.Chat(ctx, l.buildChatRequest(req, true), func(r api.ChatResponse) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			sc := model.StreamChunk{Delta: r.Message.Content}
			if r.Done {
				sc.FinishReason = r.DoneReason
				if r.EvalCount > 0 {
					total := int(math.Ceil(float64(len([]rune(r.Message.Content))) / 4))
					sc.Usage = &model.Usage{
						PromptTokens:     r.PromptEvalCount,
						CompletionTokens: r.EvalCount,
						TotalTokens:      r.PromptEvalCount + r.EvalCount,
					}
					_ = total
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- sc:
			}
			return nil
		})
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == maxRetries || !isRetryableError(err) {
			break
		}
		l.sleepBackoff(ctx, backoff, attempt)
	}
	return lastErr
}

func (l *OllamaLLM) buildChatRequest(req model.ChatRequest, stream bool) *api.ChatRequest {
	modelName := req.Model
	if modelName == "" {
		modelName = l.cfg.Model
	}
	msgs := make([]api.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, api.Message{Role: m.Role, Content: m.Content})
	}
	return &api.ChatRequest{
		Model:    modelName,
		Messages: msgs,
		Stream:   &stream,
		Options: map[string]any{
			"temperature": l.cfg.Temperature,
			"num_predict": l.cfg.MaxTokens,
		},
	}
}

func (l *OllamaLLM) toChatResponse(r api.ChatResponse) model.ChatResponse {
	usage := model.Usage{}
	if r.PromptEvalCount > 0 || r.EvalCount > 0 {
		usage.PromptTokens = r.PromptEvalCount
		usage.CompletionTokens = r.EvalCount
		usage.TotalTokens = r.PromptEvalCount + r.EvalCount
	} else {
		usage = estimateUsage(r.Message.Content)
		l.logger.Debug("ollama usage missing, estimated", zap.Int("completion", usage.CompletionTokens))
	}
	return model.ChatResponse{
		Content:      r.Message.Content,
		Model:        r.Model,
		Usage:        usage,
		FinishReason: r.DoneReason,
	}
}

func (l *OllamaLLM) sleepBackoff(ctx context.Context, base time.Duration, attempt int) {
	sleep := base * (1 << attempt)
	sleep += time.Duration(rand.Int63n(int64(sleep)/2+1)) - time.Duration(int64(sleep)/4)
	if sleep > 30*time.Second {
		sleep = 30 * time.Second
	}
	timer := time.NewTimer(sleep)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
	}
}

func (l *OllamaLLM) Ping(ctx context.Context) error {
	_, err := l.Generate(ctx, model.ChatRequest{Messages: []model.ChatMessage{{Role: "user", Content: "ping"}}})
	return err
}

func (l *OllamaLLM) BreakerState() string { return stateString(l.breaker.State()) }

func (l *OllamaLLM) ProviderName() string { return "ollama" }
func (l *OllamaLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*OllamaLLM)(nil)

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
