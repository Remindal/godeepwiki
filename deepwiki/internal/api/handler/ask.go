package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/service"
)

// AskHandler POST /api/v1/ask 与 /ask/stream。
type AskHandler struct {
	svc    *service.AskService
	logger *zap.Logger
}

func NewAskHandler(svc *service.AskService, logger *zap.Logger) *AskHandler {
	return &AskHandler{svc: svc, logger: logger}
}

func (h *AskHandler) Ask(c *gin.Context) {
	// TODO: POST /api/v1/ask（§6.2）：ShouldBindJSON dto.AskRequest + 校验
	// （repo_id 格式；question 长度 1~4000；mode ∈ keyword|embedding|hybrid；top_k 1~30）→ 40001 + details；
	// 成功 200 + dto.AskResponse；仓库非 ready → 40902；LLM 不可用 → 50201；Embedding 不可用 → 50202；
	// OpenSearch 不可用 → 50303；pgvector 查询失败 → 50203；限流桶 L1 per-IP + L2 ask_per_minute。
	respondNotImplemented(c)
}

func (h *AskHandler) AskStream(c *gin.Context) {
	// TODO: POST /api/v1/ask/stream（§6.3 SSE）：
	// ① 响应头 Content-Type: text/event-stream、Cache-Control: no-cache、Connection: keep-alive、X-Accel-Buffering: no；
	// ② 事件 id 为连接内单调递增序号；每 15s 一行 ": heartbeat" 心跳；
	// ③ 事件顺序 references（恰好 1）→ token（0~N）→ done（恰好 1）；error 任意位置终止流；
	// ④ 客户端断开 → ctx 取消 → 中断 LLM 流并退出 goroutine（硬约束 #4）；
	// ⑤ 校验失败在升级 SSE 前返回 40001 信封。
	respondNotImplemented(c)
}
