package llm

import (
	"context"
	"fmt"
	"iter"
	"math"
	"strings"

	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
	"google.golang.org/genai"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// GeminiLLM 基于 google.golang.org/genai 的 Gemini 对话实现（变更总纲 §4.7）。
type GeminiLLM struct {
	cfg     config.LLMConfig
	client  *genai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewGeminiLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *GeminiLLM {
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(fmt.Sprintf("genai client init: %v", err))
	}
	return &GeminiLLM{
		cfg:     cfg,
		client:  client,
		breaker: breaker,
		logger:  logger,
	}
}

func (l *GeminiLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	modelName := req.Model
	if modelName == "" {
		modelName = l.cfg.Model
	}
	contents, cfg := l.toGenAIContents(req)
	result, err := l.breaker.Execute(func() (any, error) {
		return l.client.Models.GenerateContent(ctx, modelName, contents, cfg)
	})
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("llm unavailable: %w", err)
	}
	return l.toChatResponse(result.(*genai.GenerateContentResponse)), nil
}

func (l *GeminiLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	modelName := req.Model
	if modelName == "" {
		modelName = l.cfg.Model
	}
	contents, cfg := l.toGenAIContents(req)
	result, err := l.breaker.Execute(func() (any, error) {
		return l.client.Models.GenerateContentStream(ctx, modelName, contents, cfg), nil
	})
	if err != nil {
		return nil, fmt.Errorf("llm unavailable: %w", err)
	}
	seq := result.(iter.Seq2[*genai.GenerateContentResponse, error])

	ch := make(chan model.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("gemini stream panic", zap.Any("panic", r))
			}
			close(ch)
		}()
		var full string
		var finishReason string
		var usage *model.Usage
		for resp, err := range seq {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				select {
				case <-ctx.Done():
				case ch <- model.StreamChunk{Err: fmt.Errorf("llm stream error: %w", err)}:
				}
				return
			}
			delta := ""
			if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
				delta = contentText(resp.Candidates[0].Content)
				finishReason = string(resp.Candidates[0].FinishReason)
			}
			full += delta
			if resp.UsageMetadata != nil {
				usage = &model.Usage{
					PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
					CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
					TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
				}
			}
			select {
			case <-ctx.Done():
				return
			case ch <- model.StreamChunk{Delta: delta}:
			}
		}
		if usage == nil && full != "" {
			total := int(math.Ceil(float64(len([]rune(full))) / 4))
			usage = &model.Usage{CompletionTokens: total, TotalTokens: total}
		}
		select {
		case <-ctx.Done():
		case ch <- model.StreamChunk{FinishReason: finishReason, Usage: usage}:
		}
	}()
	return ch, nil
}

func (l *GeminiLLM) toGenAIContents(req model.ChatRequest) ([]*genai.Content, *genai.GenerateContentConfig) {
	var contents []*genai.Content
	var systemText string
	for _, m := range req.Messages {
		if m.Role == "system" {
			if systemText != "" {
				systemText += "\n"
			}
			systemText += m.Content
			continue
		}
		role := genai.RoleUser
		if m.Role == "assistant" {
			role = genai.RoleModel
		}
		contents = append(contents, genai.NewContentFromText(m.Content, genai.Role(role)))
	}
	cfg := &genai.GenerateContentConfig{}
	if systemText != "" {
		cfg.SystemInstruction = genai.NewContentFromText(systemText, "")
	}
	if l.cfg.Temperature != 0 {
		temp := float32(l.cfg.Temperature)
		cfg.Temperature = &temp
	}
	maxTok := int32(l.cfg.MaxTokens)
	if req.MaxTokens > 0 {
		maxTok = int32(req.MaxTokens)
	}
	if maxTok > 0 {
		cfg.MaxOutputTokens = maxTok
	}
	return contents, cfg
}

func (l *GeminiLLM) toChatResponse(resp *genai.GenerateContentResponse) model.ChatResponse {
	content := ""
	finishReason := ""
	if len(resp.Candidates) > 0 {
		c := resp.Candidates[0]
		if c.Content != nil {
			content = contentText(c.Content)
		}
		finishReason = string(c.FinishReason)
	}
	usage := model.Usage{}
	if resp.UsageMetadata != nil {
		usage.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.CompletionTokens = int(resp.UsageMetadata.CandidatesTokenCount)
		usage.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
	} else {
		usage = estimateUsage(content)
		l.logger.Debug("gemini usage missing, estimated", zap.Int("completion", usage.CompletionTokens))
	}
	return model.ChatResponse{
		Content:      content,
		Model:        l.cfg.Model,
		Usage:        usage,
		FinishReason: finishReason,
	}
}

func contentText(c *genai.Content) string {
	var parts []string
	for _, p := range c.Parts {
		if p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "")
}

func (l *GeminiLLM) Ping(ctx context.Context) error {
	_, err := l.Generate(ctx, model.ChatRequest{Messages: []model.ChatMessage{{Role: "user", Content: "ping"}}})
	return err
}

func (l *GeminiLLM) BreakerState() string { return stateString(l.breaker.State()) }

func (l *GeminiLLM) ProviderName() string { return "gemini" }
func (l *GeminiLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*GeminiLLM)(nil)
