package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

func newOpenAIClient(cfg config.LLMConfig, defaultBaseURL string) openai.Client {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return openai.NewClient(opts...)
}

func toOpenAIMessages(messages []model.ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "assistant":
			out = append(out, openai.AssistantMessage(m.Content))
		default:
			out = append(out, openai.UserMessage(m.Content))
		}
	}
	return out
}

func buildOpenAIChatParams(cfg config.LLMConfig, req model.ChatRequest) openai.ChatCompletionNewParams {
	modelName := req.Model
	if modelName == "" {
		modelName = cfg.Model
	}
	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = int64(cfg.MaxTokens)
	}
	temp := req.Temperature
	if temp == 0 {
		temp = cfg.Temperature
	}
	return openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(modelName),
		Messages:  toOpenAIMessages(req.Messages),
		MaxTokens: openai.Int(maxTokens),
		Temperature: openai.Float(temp),
	}
}

func generateWithOpenAI(
	ctx context.Context,
	client openai.Client,
	cfg config.LLMConfig,
	breaker *gobreaker.CircuitBreaker[any],
	logger *zap.Logger,
	req model.ChatRequest,
) (model.ChatResponse, error) {
	params := buildOpenAIChatParams(cfg, req)
	result, err := breaker.Execute(func() (any, error) {
		return client.Chat.Completions.New(ctx, params)
	})
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("llm unavailable: %w", err)
	}
	resp := result.(*openai.ChatCompletion)
	if len(resp.Choices) == 0 {
		return model.ChatResponse{}, fmt.Errorf("llm unavailable: empty choices")
	}
	choice := resp.Choices[0]
	content := choice.Message.Content
	usage := model.Usage{}
	if resp.Usage.JSON.TotalTokens.Valid() {
		usage.PromptTokens = int(resp.Usage.PromptTokens)
		usage.CompletionTokens = int(resp.Usage.CompletionTokens)
		usage.TotalTokens = int(resp.Usage.TotalTokens)
	} else {
		usage = estimateUsage(content)
		logger.Debug("openai usage missing, estimated", zap.Int("prompt", usage.PromptTokens), zap.Int("completion", usage.CompletionTokens))
	}
	return model.ChatResponse{
		Content:      content,
		Model:        resp.Model,
		Usage:        usage,
		FinishReason: choice.FinishReason,
	}, nil
}

func streamWithOpenAI(
	ctx context.Context,
	client openai.Client,
	cfg config.LLMConfig,
	breaker *gobreaker.CircuitBreaker[any],
	logger *zap.Logger,
	req model.ChatRequest,
) (<-chan model.StreamChunk, error) {
	params := buildOpenAIChatParams(cfg, req)
	// 流式建立经 breaker 包裹，流内错误也计入 breaker 失败。
	result, err := breaker.Execute(func() (any, error) {
		return client.Chat.Completions.NewStreaming(ctx, params), nil
	})
	if err != nil {
		return nil, fmt.Errorf("llm unavailable: %w", err)
	}
	stream := result.(*ssestream.Stream[openai.ChatCompletionChunk])

	ch := make(chan model.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("openai stream panic", zap.Any("panic", r))
			}
			close(ch)
		}()
		for stream.Next() {
			if ctx.Err() != nil {
				return
			}
			chunk := stream.Current()
			sc := model.StreamChunk{}
			if len(chunk.Choices) > 0 {
				d := chunk.Choices[0].Delta
				sc.Delta = d.Content
				if sc.Delta == "" {
					// DeepSeek-V4 等 thinking 模型的推理阶段经 delta.reasoning_content 下发
					//（openai-go 未建模该扩展字段，落 ExtraFields；不转发会让客户端长时间无 token）。
					if f, ok := d.JSON.ExtraFields["reasoning_content"]; ok && f.Valid() {
						var rs string
						if json.Unmarshal([]byte(f.Raw()), &rs) == nil {
							sc.Delta = rs
						}
					}
				}
				sc.FinishReason = chunk.Choices[0].FinishReason
			}
			if chunk.Usage.JSON.TotalTokens.Valid() {
				sc.Usage = &model.Usage{
					PromptTokens:     int(chunk.Usage.PromptTokens),
					CompletionTokens: int(chunk.Usage.CompletionTokens),
					TotalTokens:      int(chunk.Usage.TotalTokens),
				}
			}
			select {
			case <-ctx.Done():
				return
			case ch <- sc:
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case <-ctx.Done():
			case ch <- model.StreamChunk{Err: fmt.Errorf("llm stream error: %w", err)}:
			}
		}
	}()
	return ch, nil
}

func estimateUsage(content string) model.Usage {
	comp := int(math.Ceil(float64(len([]rune(content))) / 4))
	return model.Usage{CompletionTokens: comp, TotalTokens: comp}
}

func stateString(s gobreaker.State) string {
	switch s {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
