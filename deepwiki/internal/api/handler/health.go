// Package handler Handler 层：只做参数绑定/校验、统一信封装配、错误码映射；
// 不直接访问 DB / Provider，只调 Service（基线 §2.2）。
package handler

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/api/middleware"
	"deepwiki/internal/config"
	"deepwiki/internal/model"
	"deepwiki/internal/observability"
	"deepwiki/internal/task"
)

// ---------- 包内共享响应辅助（统一信封，硬约束 #8） ----------

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, model.Envelope{
		Code:      0,
		Message:   "ok",
		Data:      data,
		RequestID: middleware.GetRequestID(c),
	})
}

func respondError(c *gin.Context, code int, message string, details []model.ErrorDetail) {
	c.JSON(model.HTTPStatusOf(code), model.Envelope{
		Code:      code,
		Message:   message,
		RequestID: middleware.GetRequestID(c),
		Details:   details,
	})
}

// ---------- HealthSnapshot（60s 后台探测循环写，health 接口只读，毫秒级返回） ----------

// HealthSnapshot 依赖状态快照容器（atomic.Value 持有 dto.HealthResponse 的依赖字段部分；
// 总纲 §7：探测仍走 60s 后台缓存，health 接口本身禁止发起外部调用）。
type HealthSnapshot struct {
	mu   sync.RWMutex
	data dto.HealthResponse
}

func NewHealthSnapshot() *HealthSnapshot {
	return &HealthSnapshot{data: dto.HealthResponse{
		Status: "degraded",
		Redis:  dto.RedisHealth{Mode: "sentinel"},
	}}
}

// Store 覆盖依赖字段（Version/UptimeSeconds/StartedAt/Worker 由 handler 每次请求时现填）。
func (s *HealthSnapshot) Store(d dto.HealthResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = d
}

func (s *HealthSnapshot) Load() dto.HealthResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// ---------- HealthHandler ----------

// HealthHandler GET /api/v1/health（总纲 §7，建议⑬）。
type HealthHandler struct {
	version string
	start   time.Time
	ready   *atomic.Bool
	cfg     *config.Manager
	snap    *HealthSnapshot
	worker  func() task.WorkerStats
	logger  *zap.Logger
}

func NewHealthHandler(version string, start time.Time, ready *atomic.Bool, cfg *config.Manager, snap *HealthSnapshot, worker func() task.WorkerStats, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{version: version, start: start, ready: ready, cfg: cfg, snap: snap, worker: worker, logger: logger}
}

func (h *HealthHandler) Health(c *gin.Context) {
	cfg := h.cfg.Get()
	// 依赖字段全部来自 60s 后台探测缓存（毫秒级返回；接口内禁止发起外部调用）。
	data := h.snap.Load()
	data.Version = h.version
	data.UptimeSeconds = int64(time.Since(h.start).Seconds())
	data.StartedAt = h.start.UTC().Format(time.RFC3339)
	data.LLM.Provider = cfg.LLM.Provider
	data.LLM.Model = cfg.LLM.Model
	data.Embedding.Provider = cfg.Embedding.Provider
	data.Embedding.Model = cfg.Embedding.Model
	ws := h.worker()
	data.Worker = dto.WorkerHealth{Busy: ws.Busy, Total: ws.Total, Queued: ws.Queued}
	observability.SetWorkerBusy(float64(ws.Busy))
	if !h.ready.Load() { // 优雅退出中：503 + 50301，status 保持原值供诊断（§6.6）
		c.JSON(http.StatusServiceUnavailable, model.Envelope{
			Code:      model.CodeServiceNotReady,
			Message:   model.MessageOf(model.CodeServiceNotReady),
			Data:      data,
			RequestID: middleware.GetRequestID(c),
		})
		return
	}
	respondOK(c, data)
}
