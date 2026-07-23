package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/api/middleware"
	"deepwiki/internal/model"
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
	var req dto.IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, model.CodeInvalidParam, "invalid json body", []model.ErrorDetail{{Field: "body", Issue: err.Error()}})
		return
	}

	var details []model.ErrorDetail
	details = append(details, validateRepoURL(req.RepoURL)...)
	details = append(details, validateBranch(req.Branch)...)
	details = append(details, validateIngestOptions(req.Options)...)
	if len(details) > 0 {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), details)
		return
	}

	t, repo, err := h.svc.SubmitIngest(c.Request.Context(), req)
	if err != nil {
		h.handleSubmitError(c, err)
		return
	}

	respondAccepted(c, dto.TaskSubmittedResponse{
		TaskID:        t.TaskID,
		RepoID:        repo.RepoID,
		Type:          string(t.Type),
		State:         string(t.State),
		QueuePosition: t.QueuePosition,
		CreatedAt:     t.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *IngestHandler) handleSubmitError(c *gin.Context, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	switch {
	case errors.Is(err, model.ErrQueueFull):
		// 估算 Retry-After：clamp(10, 600)。骨架取 60s 保守值。
		c.Header("Retry-After", "60")
		respondError(c, model.CodeQueueFull, model.MessageOf(model.CodeQueueFull), nil)
	case errors.Is(err, model.ErrQueueUnavailable):
		respondError(c, model.CodeQueueUnavailable, model.MessageOf(model.CodeQueueUnavailable), nil)
	default:
		h.logger.Error("ingest submit failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
	}
}

// respondAccepted 202 统一信封（建任务类端点）。
func respondAccepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, model.Envelope{
		Code:      0,
		Message:   "ok",
		Data:      data,
		RequestID: middleware.GetRequestID(c),
	})
}
