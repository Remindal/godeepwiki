package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/service"
)

// IngestHandler POST /api/v1/ingest。
type IngestHandler struct {
	svc    *service.IngestService
	logger *zap.Logger
}

func NewIngestHandler(svc *service.IngestService, logger *zap.Logger) *IngestHandler {
	return &IngestHandler{svc: svc, logger: logger}
}

func (h *IngestHandler) Ingest(c *gin.Context) {
	// TODO: 实现 POST /api/v1/ingest（§6.1），要求：
	// ① ShouldBindJSON dto.IngestRequest + 校验（repo_url 合法 git URL、拒绝 file:// 等本地协议、≤512；
	//    branch ≤128 且禁止 ..、空白与 ~^:?*[\ 等 git ref 非法字符；options 字段按 §6.1 表校验）→ 失败 40001 + details；
	// ② 调 h.svc.SubmitIngest；幂等命中 40901（details 附 existing_repo_id / use_refresh）；
	// ③ 成功 202 + dto.TaskSubmittedResponse；
	// ④ model.ErrQueueFull → 429 + 42902 + Retry-After（估算：clamp(queued/pool_size×avg_task_seconds, 10, 600)，
	//    queued 为 RabbitMQ 主队列深度，§9.4）；
	// ⑤ model.ErrQueueUnavailable → 503 + 50302 queue_unavailable（RabbitMQ 投递 confirm 失败，总纲 §6）；
	// ⑥ 禁止回传 err.Error() 原文（硬约束 #8）。
	respondNotImplemented(c)
}
