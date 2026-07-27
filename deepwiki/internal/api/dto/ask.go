package dto

// ChatTurn 多轮对话的一轮（role=user|assistant）。
type ChatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AskRequest POST /api/v1/ask 与 /ask/stream 共用请求体（§6.2）。
// History 可选：多轮上下文（前端传最近 N 轮，后端按序拼入 prompt；缺省 = 单轮无状态）。
type AskRequest struct {
	RepoID      string     `json:"repo_id" binding:"required"`
	Question    string     `json:"question" binding:"required"`
	Mode        string     `json:"mode"` // keyword|embedding|hybrid，缺省取配置
	TopK        *int       `json:"top_k"`
	Temperature *float64   `json:"temperature"`
	History     []ChatTurn `json:"history,omitempty"`
}

// ReferenceDTO 引用片段（必须来自真实检索结果，硬约束 #15）。
type ReferenceDTO struct {
	ChunkID   string  `json:"chunk_id"`
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Language  string  `json:"language"`
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
}

// UsageDTO token 用量。
type UsageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// AskResponse POST /api/v1/ask 200 响应 data（§6.2）。
type AskResponse struct {
	Answer     string         `json:"answer"`
	References []ReferenceDTO `json:"references"`
	Mode       string         `json:"mode"`
	Usage      UsageDTO       `json:"usage"`
	LatencyMs  int64          `json:"latency_ms"`
}

// StreamReferencesEvent SSE references 事件 payload（§6.3）。
type StreamReferencesEvent struct {
	RequestID  string         `json:"request_id"`
	Mode       string         `json:"mode"`
	References []ReferenceDTO `json:"references"`
}

// StreamTokenEvent SSE token 事件 payload。
type StreamTokenEvent struct {
	Delta string `json:"delta"`
}

// StreamDoneEvent SSE done 事件 payload。
type StreamDoneEvent struct {
	Usage     UsageDTO `json:"usage"`
	LatencyMs int64    `json:"latency_ms"`
}

// StreamErrorEvent SSE error 事件 payload（任意阶段失败，此前已推送的事件不回滚）。
type StreamErrorEvent struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}
