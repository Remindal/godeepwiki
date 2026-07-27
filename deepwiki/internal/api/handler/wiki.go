package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/model"
	"deepwiki/internal/service"
)

// WikiHandler POST /api/v1/wiki/generate 与 GET /api/v1/repos/{repo_id}/wiki。
type WikiHandler struct {
	svc    *service.WikiService
	logger *zap.Logger
}

func NewWikiHandler(svc *service.WikiService, logger *zap.Logger) *WikiHandler {
	return &WikiHandler{svc: svc, logger: logger}
}

type wikiGenerateRequest struct {
	RepoID string `json:"repo_id" binding:"required"`
}

func (h *WikiHandler) Generate(c *gin.Context) {
	var req wikiGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, model.CodeInvalidParam, "invalid json body", []model.ErrorDetail{{Field: "body", Issue: err.Error()}})
		return
	}
	if !validRepoID(req.RepoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}

	t, err := h.svc.Generate(c.Request.Context(), req.RepoID)
	if err != nil {
		h.handleWikiError(c, err)
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

func (h *WikiHandler) GetWiki(c *gin.Context) {
	repoID := c.Param("repo_id")
	if !validRepoID(repoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}
	w, err := h.svc.GetWiki(c.Request.Context(), repoID)
	if err != nil {
		h.handleWikiError(c, err)
		return
	}
	respondOK(c, w)
}

// Export GET /api/v1/repos/{repo_id}/wiki/export：把 wiki 按目录序拼成单个 markdown 文件下载。
// 文件下载不走统一信封，直出字节流（Content-Disposition: attachment）。
func (h *WikiHandler) Export(c *gin.Context) {
	repoID := c.Param("repo_id")
	if !validRepoID(repoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}
	w, err := h.svc.GetWiki(c.Request.Context(), repoID)
	if err != nil {
		h.handleWikiError(c, err)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Wiki: %s\n\n", repoID))
	sb.WriteString(fmt.Sprintf("> 生成时间：%s · 共 %d 页\n\n", w.GeneratedAt.UTC().Format(time.RFC3339), len(w.Pages)))
	if len(w.TOC) > 0 {
		sb.WriteString("## 目录\n\n")
		for _, item := range w.TOC {
			sb.WriteString(fmt.Sprintf("- [%s](#%s)\n", item.Title, item.Slug))
		}
		sb.WriteString("\n---\n\n")
	}
	for _, p := range w.Pages {
		sb.WriteString(fmt.Sprintf("## %s\n\n<a id=\"%s\"></a>\n\n%s\n\n---\n\n", p.Title, p.Slug, p.ContentMD))
	}

	filename := fmt.Sprintf("wiki-%s.md", repoID)
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.String(http.StatusOK, sb.String())
}

func (h *WikiHandler) handleWikiError(c *gin.Context, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	switch {
	case errors.Is(err, model.ErrRepoNotFound):
		respondError(c, model.CodeRepoNotFound, model.MessageOf(model.CodeRepoNotFound), nil)
	case errors.Is(err, model.ErrWikiNotFound):
		respondError(c, model.CodeWikiNotFound, model.MessageOf(model.CodeWikiNotFound), nil)
	default:
		h.logger.Error("wiki operation failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
	}
}
