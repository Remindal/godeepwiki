package model

// ChatMessage 角色：system|user|assistant。
type ChatMessage struct {
	Role    string
	Content string
}

// ChatRequest 流式与非流式共用（建议⑧）；是否流式由调用 LLM 的哪个方法决定，结构体不含 stream 字段。
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Temperature float64
	MaxTokens   int
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ChatResponse struct {
	Content      string
	Model        string
	Usage        Usage
	FinishReason string // stop|length|content_filter|...
}

// StreamChunk 流式输出元素；消费方必须先检查 Err。
type StreamChunk struct {
	Delta        string  // 增量文本
	Reasoning    bool    // true = thinking 模型推理段（reasoning_content），前端折叠灰显
	FinishReason string  // 非空表示结束原因
	Usage        *Usage  // 仅结束 chunk 可能携带（provider 支持时）
	Err          error   // 非 nil 表示流内错误，此后 channel 将被关闭
}
