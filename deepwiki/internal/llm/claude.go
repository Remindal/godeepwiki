package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// ClaudeLLM 基于官方 anthropic-sdk-go 的 Claude 对话实现（变更总纲 §4.7）。
type ClaudeLLM struct {
	cfg     config.LLMConfig
	client  anthropic.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewClaudeLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *ClaudeLLM {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &ClaudeLLM{
		cfg:     cfg,
		client:  anthropic.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *ClaudeLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	params := l.buildParams(req)
	result, err := l.breaker.Execute(func() (any, error) {
		return l.client.Messages.New(ctx, params)
	})
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("llm unavailable: %w", err)
	}
	msg := result.(*anthropic.Message)
	content := l.extractText(msg.Content)
	usage := model.Usage{
		PromptTokens:     int(msg.Usage.InputTokens),
		CompletionTokens: int(msg.Usage.OutputTokens),
		TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
	}
	return model.ChatResponse{
		Content:      content,
		Model:        string(msg.Model),
		Usage:        usage,
		FinishReason: string(msg.StopReason),
	}, nil
}

func (l *ClaudeLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	params := l.buildParams(req)
	result, err := l.breaker.Execute(func() (any, error) {
		return l.client.Messages.NewStreaming(ctx, params), nil
	})
	if err != nil {
		return nil, fmt.Errorf("llm unavailable: %w", err)
	}
	stream := result.(*ssestream.Stream[anthropic.MessageStreamEventUnion])

	ch := make(chan model.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("claude stream panic", zap.Any("panic", r))
			}
			close(ch)
		}()
		var finishReason string
		var usage *model.Usage
		for stream.Next() {
			if ctx.Err() != nil {
				return
			}
			ev := stream.Current()
			switch ev.Type {
			case "content_block_delta":
				delta := ev.AsContentBlockDelta().Delta.Text
				select {
				case <-ctx.Done():
					return
				case ch <- model.StreamChunk{Delta: delta}:
				}
			case "message_delta":
				msgDelta := ev.AsMessageDelta()
				finishReason = string(msgDelta.Delta.StopReason)
				usage = &model.Usage{
					PromptTokens:     int(msgDelta.Usage.InputTokens),
					CompletionTokens: int(msgDelta.Usage.OutputTokens),
					TotalTokens:      int(msgDelta.Usage.InputTokens + msgDelta.Usage.OutputTokens),
				}
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case <-ctx.Done():
			case ch <- model.StreamChunk{Err: fmt.Errorf("llm stream error: %w", err)}:
			}
			return
		}
		select {
		case <-ctx.Done():
		case ch <- model.StreamChunk{FinishReason: finishReason, Usage: usage}:
		}
	}()
	return ch, nil
}

func (l *ClaudeLLM) buildParams(req model.ChatRequest) anthropic.MessageNewParams {
	modelName := req.Model
	if modelName == "" {
		modelName = l.cfg.Model
	}
	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = int64(l.cfg.MaxTokens)
	}
	var systemBlocks []anthropic.TextBlockParam
	var messages []anthropic.MessageParam
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: m.Content})
			continue
		}
		if m.Role == "assistant" {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		} else {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(modelName),
		MaxTokens: maxTokens,
		Messages:  messages,
		System:    systemBlocks,
	}
	if l.cfg.Temperature != 0 {
		params.Temperature = anthropic.Float(l.cfg.Temperature)
	}
	return params
}

func (l *ClaudeLLM) extractText(blocks []anthropic.ContentBlockUnion) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}

func (l *ClaudeLLM) Ping(ctx context.Context) error {
	_, err := l.Generate(ctx, model.ChatRequest{Messages: []model.ChatMessage{{Role: "user", Content: "ping"}}})
	return err
}

func (l *ClaudeLLM) BreakerState() string { return stateString(l.breaker.State()) }

func (l *ClaudeLLM) ProviderName() string { return "claude" }
func (l *ClaudeLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*ClaudeLLM)(nil)
