package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/llm"
	"deepwiki/internal/model"
	"deepwiki/internal/observability"
	"deepwiki/internal/retriever"
	"deepwiki/internal/store"
)

// AskService 问答编排（§6.2、§6.3；Ask 不得直接访问 pgvector/OpenSearch，只面向 Retriever 接口，建议⑥）。
type AskService struct {
	cfg        *config.Manager
	repos      store.RepoStore
	retrievers map[string]retriever.Retriever // key = mode：keyword|embedding|hybrid
	llm        llm.LLM
	logger     *zap.Logger
}

func NewAskService(cfg *config.Manager, repos store.RepoStore, retrievers map[string]retriever.Retriever, l llm.LLM, logger *zap.Logger) *AskService {
	return &AskService{cfg: cfg, repos: repos, retrievers: retrievers, llm: l, logger: logger}
}

// Ask POST /api/v1/ask（§6.2）。
func (s *AskService) Ask(ctx context.Context, req dto.AskRequest) (*dto.AskResponse, error) {
	start := time.Now()

	mode, topK, temperature, err := s.normalizeAskParams(req)
	if err != nil {
		observability.IncAsk("failure")
		return nil, err
	}
	if err := s.ensureRepoReady(ctx, req.RepoID); err != nil {
		observability.IncAsk("failure")
		return nil, err
	}

	hits, err := s.search(ctx, mode, req.RepoID, req.Question, topK)
	if err != nil {
		observability.IncAsk("failure")
		return nil, err
	}
	references := buildReferences(hits)

	prompt := buildAskPrompt(req.Question, hits, req.History)
	resp, err := s.llm.Generate(ctx, model.ChatRequest{
		Model:       s.llm.ModelName(),
		Messages:    prompt,
		Temperature: temperature,
		MaxTokens:   s.cfg.Get().LLM.MaxTokens,
	})
	if err != nil {
		observability.IncAsk("failure")
		return nil, mapLLMError(err)
	}

	observability.IncAsk("success")
	usage := usageFromResponse(resp)
	return &dto.AskResponse{
		Answer:     resp.Content,
		References: references,
		Mode:       mode,
		Usage:      usage,
		LatencyMs:  time.Since(start).Milliseconds(),
	}, nil
}

// AskStream POST /api/v1/ask/stream 的业务实现（§6.3；sink 由 handler 提供，负责 SSE 帧写出）。
func (s *AskService) AskStream(ctx context.Context, req dto.AskRequest, sink func(event string, payload any) error) error {
	start := time.Now()

	mode, topK, temperature, err := s.normalizeAskParams(req)
	if err != nil {
		observability.IncAsk("failure")
		return err
	}
	if err := s.ensureRepoReady(ctx, req.RepoID); err != nil {
		observability.IncAsk("failure")
		return err
	}

	hits, err := s.search(ctx, mode, req.RepoID, req.Question, topK)
	if err != nil {
		observability.IncAsk("failure")
		return err
	}
	if err := sink("references", dto.StreamReferencesEvent{Mode: mode, References: buildReferences(hits)}); err != nil {
		return err
	}

	prompt := buildAskPrompt(req.Question, hits, req.History)
	stream, err := s.llm.GenerateStream(ctx, model.ChatRequest{
		Model:       s.llm.ModelName(),
		Messages:    prompt,
		Temperature: temperature,
		MaxTokens:   s.cfg.Get().LLM.MaxTokens,
	})
	if err != nil {
		observability.IncAsk("failure")
		return mapLLMError(err)
	}

	var answer strings.Builder
	var usage *model.Usage
	for chunk := range stream {
		if chunk.Err != nil {
			observability.IncAsk("failure")
			return mapLLMError(chunk.Err)
		}
		if chunk.Delta != "" {
			if chunk.Reasoning {
				// 推理段走独立 thinking 事件（不进正式答案，前端折叠灰显）。
				if err := sink("thinking", dto.StreamTokenEvent{Delta: chunk.Delta}); err != nil {
					return err
				}
				continue
			}
			answer.WriteString(chunk.Delta)
			if err := sink("token", dto.StreamTokenEvent{Delta: chunk.Delta}); err != nil {
				return err
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	u := usageFromStream(usage, answer.String())
	if err := sink("done", dto.StreamDoneEvent{Usage: u, LatencyMs: time.Since(start).Milliseconds()}); err != nil {
		return err
	}
	observability.IncAsk("success")
	return nil
}

func (s *AskService) normalizeAskParams(req dto.AskRequest) (mode string, topK int, temperature float64, err error) {
	cfg := s.cfg.Get()
	mode = req.Mode
	if mode == "" {
		mode = cfg.Retriever.Mode
	}
	if _, ok := s.retrievers[mode]; !ok {
		return "", 0, 0, model.NewAPIError(model.CodeInvalidParam, "invalid mode")
	}
	topK = cfg.Retriever.TopK
	if req.TopK != nil {
		topK = *req.TopK
	}
	if topK < 1 {
		topK = 1
	}
	if topK > 30 {
		topK = 30
	}
	temperature = cfg.LLM.Temperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	if temperature < 0 {
		temperature = 0
	}
	if temperature > 2 {
		temperature = 2
	}
	return mode, topK, temperature, nil
}

func (s *AskService) ensureRepoReady(ctx context.Context, repoID string) error {
	repo, err := s.repos.Get(ctx, repoID)
	if err != nil {
		if errors.Is(err, model.ErrRepoNotFound) {
			return model.ErrRepoNotFound
		}
		return err
	}
	if repo.State != "ready" {
		return model.NewAPIError(model.CodeInvalidTaskState, "repo is not ready")
	}
	return nil
}

func (s *AskService) search(ctx context.Context, mode, repoID, query string, topK int) ([]model.ChunkHit, error) {
	r, ok := s.retrievers[mode]
	if !ok {
		return nil, model.NewAPIError(model.CodeInvalidParam, "invalid mode")
	}
	hits, err := r.Search(ctx, repoID, query, topK)
	if err != nil {
		var apiErr *model.APIError
		if errors.As(err, &apiErr) {
			return nil, apiErr
		}
		if mode == "embedding" {
			return nil, &model.APIError{Code: model.CodeVectorStoreUnavailable, Message: model.MessageOf(model.CodeVectorStoreUnavailable), Err: err}
		}
		return nil, &model.APIError{Code: model.CodeSearchUnavailable, Message: model.MessageOf(model.CodeSearchUnavailable), Err: err}
	}
	return hits, nil
}

func buildReferences(hits []model.ChunkHit) []dto.ReferenceDTO {
	refs := make([]dto.ReferenceDTO, 0, len(hits))
	for _, h := range hits {
		refs = append(refs, dto.ReferenceDTO{
			ChunkID:   h.Chunk.ChunkID,
			Path:      h.Chunk.Path,
			StartLine: h.Chunk.StartLine,
			EndLine:   h.Chunk.EndLine,
			Language:  h.Chunk.Language,
			Score:     h.Score,
			Snippet:   truncateRunes(h.Chunk.Content, 300),
		})
	}
	return refs
}

// buildAskPrompt 组装 prompt：system 规则 → 多轮历史（可选，最近 6 轮，单条截断 500 runes）
// → 当前代码片段 + 问题。检索仍只基于当前问题（query 重写留待后续）。
func buildAskPrompt(question string, hits []model.ChunkHit, history []dto.ChatTurn) []model.ChatMessage {
	system := `你是一个代码问答助手。你只能依据下面给出的代码片段回答问题。
引用格式必须使用 [path:start-end]；禁止编造行号与文件路径。
如果片段不足以回答，请回答“未在仓库中找到相关代码”。`

	var ctx strings.Builder
	for _, h := range hits {
		ctx.WriteString(fmt.Sprintf("\n--- %s:%d-%d ---\n%s\n", h.Chunk.Path, h.Chunk.StartLine, h.Chunk.EndLine, h.Chunk.Content))
	}

	messages := []model.ChatMessage{{Role: "system", Content: system}}
	// 只保留最近 6 轮，控制 prompt token 膨胀（每轮单条截断 500 runes）。
	if len(history) > 6 {
		history = history[len(history)-6:]
	}
	for _, turn := range history {
		role := "user"
		if turn.Role == "assistant" {
			role = "assistant"
		}
		messages = append(messages, model.ChatMessage{Role: role, Content: truncateRunes(turn.Content, 500)})
	}

	user := fmt.Sprintf("代码片段：%s\n\n问题：%s", ctx.String(), question)
	return append(messages, model.ChatMessage{Role: "user", Content: user})
}

func usageFromResponse(resp model.ChatResponse) dto.UsageDTO {
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		return dto.UsageDTO{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens}
	}
	return estimateUsage(resp.Content, "")
}

func usageFromStream(usage *model.Usage, answer string) dto.UsageDTO {
	if usage != nil && (usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		return dto.UsageDTO{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens}
	}
	return estimateUsage(answer, "")
}

func estimateUsage(completion, prompt string) dto.UsageDTO {
	return dto.UsageDTO{
		PromptTokens:     (utf8.RuneCountInString(prompt) + 3) / 4,
		CompletionTokens: (utf8.RuneCountInString(completion) + 3) / 4,
	}
}

func mapLLMError(err error) error {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return &model.APIError{Code: model.CodeLLMUnavailable, Message: model.MessageOf(model.CodeLLMUnavailable), Err: err}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
