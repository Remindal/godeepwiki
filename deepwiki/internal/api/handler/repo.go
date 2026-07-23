package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/model"
	"deepwiki/internal/service"
)

// RepoHandler /api/v1/repos 资源族。
type RepoHandler struct {
	repos  *service.RepoService
	ingest *service.IngestService
	logger *zap.Logger
}

func NewRepoHandler(repos *service.RepoService, ingest *service.IngestService, logger *zap.Logger) *RepoHandler {
	return &RepoHandler{repos: repos, ingest: ingest, logger: logger}
}

func (h *RepoHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	repos, total, err := h.repos.ListRepos(c.Request.Context(), page, pageSize)
	if err != nil {
		h.logger.Error("list repos failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)
	respondOK(c, dto.PageResult[*model.Repo]{
		Items: repos,
		Pagination: dto.Pagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *RepoHandler) Get(c *gin.Context) {
	repoID := c.Param("repo_id")
	if !validRepoID(repoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}
	detail, err := h.repos.GetRepo(c.Request.Context(), repoID)
	if err != nil {
		h.handleRepoError(c, err)
		return
	}
	respondOK(c, detail)
}

func (h *RepoHandler) Delete(c *gin.Context) {
	repoID := c.Param("repo_id")
	if !validRepoID(repoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}
	res, err := h.repos.DeleteRepo(c.Request.Context(), repoID)
	if err != nil {
		h.handleRepoError(c, err)
		return
	}
	respondOK(c, gin.H{
		"repo_id": repoID,
		"deleted": res,
	})
}

func (h *RepoHandler) Refresh(c *gin.Context) {
	repoID := c.Param("repo_id")
	if !validRepoID(repoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}
	t, err := h.ingest.SubmitRefresh(c.Request.Context(), repoID)
	if err != nil {
		h.handleSubmitError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, model.Envelope{
		Code:    0,
		Message: "ok",
		Data: dto.TaskSubmittedResponse{
			TaskID:        t.TaskID,
			RepoID:        t.RepoID,
			Type:          string(t.Type),
			State:         string(t.State),
			QueuePosition: t.QueuePosition,
			CreatedAt:     t.CreatedAt.UTC().Format(time.RFC3339),
		},
		RequestID: requestID(c),
	})
}

func (h *RepoHandler) handleRepoError(c *gin.Context, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	switch {
	case errors.Is(err, model.ErrRepoNotFound):
		respondError(c, model.CodeRepoNotFound, model.MessageOf(model.CodeRepoNotFound), nil)
	default:
		h.logger.Error("repo operation failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
	}
}

func (h *RepoHandler) handleSubmitError(c *gin.Context, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	switch {
	case errors.Is(err, model.ErrQueueFull):
		c.Header("Retry-After", "60")
		respondError(c, model.CodeQueueFull, model.MessageOf(model.CodeQueueFull), nil)
	case errors.Is(err, model.ErrQueueUnavailable):
		respondError(c, model.CodeQueueUnavailable, model.MessageOf(model.CodeQueueUnavailable), nil)
	default:
		h.logger.Error("refresh submit failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
	}
}

func requestID(c *gin.Context) string { return c.GetString("request_id") }
