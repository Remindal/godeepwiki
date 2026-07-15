// Package llm 对话模型 Provider 抽象与实现。
package llm

import (
	"context"

	"deepwiki/internal/model"
)

// LLM 对话模型抽象（基线 §7，冻结签名）。
// 任何官方 SDK 类型禁止出现在本签名与返回值中（硬约束 #17）。
type LLM interface {
	// Generate 非流式。
	Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error)
	// GenerateStream 流式；返回 channel 由实现方关闭；流内错误通过 StreamChunk.Err 传递。
	GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error)
	ProviderName() string // openai|gemini|claude|ollama|deepseek
	ModelName() string
}
