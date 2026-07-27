package embed

import (
	"context"
	"fmt"
	"sort"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// newOpenAIClient 构造 openai-go 客户端（openai/dashscope/siliconflow/voyage 共用）。
func newOpenAIClient(cfg config.EmbeddingConfig, defaultBaseURL string) openai.Client {
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

// embedWithOpenAI 使用 openai-go 批量向量化；dims 指针用于未知模型时探测维度。
// maxRunes 单条输入 rune 上限（0=不截断）：bge-large-zh 等模型输入上限 512 tokens，
// CJK 字符约 1 token/字，超限会被 provider 400 拒绝（反 AI 错误 #14 输入侧防线）。
func embedWithOpenAI(
	ctx context.Context,
	client openai.Client,
	modelName string,
	dims *int,
	batchSize int,
	breaker *gobreaker.CircuitBreaker[any],
	logger *zap.Logger,
	texts []string,
	maxRunes int,
) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	texts = clampEmbeddingInputs(texts, maxRunes, logger)
	if batchSize <= 0 {
		batchSize = 64
	}

	result, err := breaker.Execute(func() (any, error) {
		var all [][]float32
		for start := 0; start < len(texts); start += batchSize {
			end := start + batchSize
			if end > len(texts) {
				end = len(texts)
			}
			batch := texts[start:end]
			resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
				Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: batch},
				Model: openai.EmbeddingModel(modelName),
			})
			if err != nil {
				return nil, err
			}
			vecs := make([][]float32, len(batch))
			for _, d := range resp.Data {
				idx := int(d.Index)
				if idx < 0 || idx >= len(batch) {
					continue
				}
				vec := make([]float32, len(d.Embedding))
				for i, v := range d.Embedding {
					vec[i] = float32(v)
				}
				vecs[idx] = vec
			}
			all = append(all, vecs...)
		}
		if dims != nil && *dims == 0 && len(all) > 0 && len(all[0]) > 0 {
			*dims = len(all[0])
			logger.Info("embedding dimensions probed", zap.Int("dims", *dims))
		}
		return all, nil
	})
	if err != nil {
		return nil, fmt.Errorf("embedding unavailable: %w", err)
	}
	return result.([][]float32), nil
}

// clampEmbeddingInputs 截断超长输入（按 rune；截断记 WARN，不中断流程）。
func clampEmbeddingInputs(texts []string, maxRunes int, logger *zap.Logger) []string {
	if maxRunes <= 0 {
		return texts
	}
	out := make([]string, len(texts))
	truncated := 0
	for i, t := range texts {
		r := []rune(t)
		if len(r) > maxRunes {
			out[i] = string(r[:maxRunes])
			truncated++
		} else {
			out[i] = t
		}
	}
	if truncated > 0 {
		logger.Warn("embedding inputs truncated to provider token cap",
			zap.Int("truncated", truncated), zap.Int("total", len(texts)), zap.Int("max_runes", maxRunes))
	}
	return out
}

// fallbackDimensions 按模型名兜底维度（仅用于返回 Dimensions()，真实维度以首次请求探测为准）。
func fallbackDimensions(provider, model string) int {
	switch provider {
	case "openai":
		if d, ok := openAIDims[model]; ok {
			return d
		}
	case "dashscope":
		if d, ok := dashscopeDims[model]; ok {
			return d
		}
	case "siliconflow":
		if d, ok := siliconflowDims[model]; ok {
			return d
		}
	case "voyage":
		if d, ok := voyageDims[model]; ok {
			return d
		}
	}
	return 0
}

// sortByIndex 辅助：openai-go 返回的 Data 可能按索引顺序，保险起见按 Index 排序。
func sortByIndex(data []openai.Embedding) {
	sort.Slice(data, func(i, j int) bool { return data[i].Index < data[j].Index })
}

// stateString 把 gobreaker.State 映射为 health 字符串。
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

// _ 占位，避免后续若 fallbackDimensions 未使用被误删。
var _ = sortByIndex
