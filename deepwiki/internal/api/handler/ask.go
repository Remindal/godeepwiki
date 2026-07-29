package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/api/middleware"
	"deepwiki/internal/model"
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
	var req dto.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, model.CodeInvalidParam, "invalid json body", []model.ErrorDetail{{Field: "body", Issue: err.Error()}})
		return
	}
	if details := validateAskRequest(req); len(details) > 0 {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), details)
		return
	}

	resp, err := h.svc.Ask(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	respondOK(c, resp)
}

func (h *AskHandler) AskStream(c *gin.Context) {
	var req dto.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, model.CodeInvalidParam, "invalid json body", []model.ErrorDetail{{Field: "body", Issue: err.Error()}})
		return
	}
	if details := validateAskRequest(req); len(details) > 0 {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), details)
		return
	}

	// SSE 响应头。
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, model.CodeInternalError, "streaming not supported", nil)
		return
	}

	ctx := c.Request.Context()
	var mu sync.Mutex
	var eventID int64
	writeEvent := func(event string, payload any) error {
		mu.Lock()
		defer mu.Unlock()
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		eventID++
		_, err = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", eventID, event, data)
		if err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	// 心跳 goroutine：每 15s 一行注释心跳。
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatDone:
				return
			case <-ticker.C:
				mu.Lock()
				_, _ = c.Writer.WriteString(": heartbeat\n\n")
				flusher.Flush()
				mu.Unlock()
			}
		}
	}()
	defer close(heartbeatDone)

	if err := h.svc.AskStream(ctx, req, writeEvent); err != nil {
		// 若已经开始流式输出，则以 error 事件终止；否则返回错误信封。
		if eventID > 0 {
			_ = writeEvent("error", dto.StreamErrorEvent{Code: errorCodeOf(err), Message: errorMessageOf(err), RequestID: middleware.GetRequestID(c)})
			return
		}
		h.handleError(c, err)
		return
	}
}

func validateAskRequest(req dto.AskRequest) []model.ErrorDetail {
	var details []model.ErrorDetail
	// repo_id 与 repo_url 二选一；repo_id 非空时校验格式。
	if req.RepoID == "" && req.RepoURL == "" {
		details = append(details, model.ErrorDetail{Field: "repo_id", Issue: "repo_id or repo_url is required"})
	} else if req.RepoID != "" && !validRepoID(req.RepoID) {
		details = append(details, invalidRepoIDDetail("repo_id")...)
	}
	if req.Question == "" || len(req.Question) > 4000 {
		details = append(details, model.ErrorDetail{Field: "question", Issue: "length must be between 1 and 4000"})
	}
	if req.Mode != "" && req.Mode != "keyword" && req.Mode != "embedding" && req.Mode != "hybrid" {
		details = append(details, model.ErrorDetail{Field: "mode", Issue: "must be one of keyword|embedding|hybrid"})
	}
	if req.TopK != nil && (*req.TopK < 1 || *req.TopK > 30) {
		details = append(details, model.ErrorDetail{Field: "top_k", Issue: "must be between 1 and 30"})
	}
	details = append(details, validatePathFilter(req.PathFilter)...)
	return details
}

// validatePathFilter 路径前缀过滤的格式校验（40001）：禁止 ..（防穿越）、反斜杠、前导 /、超长。
func validatePathFilter(p string) []model.ErrorDetail {
	if p == "" {
		return nil
	}
	if len(p) > 256 {
		return []model.ErrorDetail{{Field: "path_filter", Issue: "length must be <= 256"}}
	}
	if strings.Contains(p, "..") {
		return []model.ErrorDetail{{Field: "path_filter", Issue: "must not contain '..'"}}
	}
	if strings.Contains(p, "\\") {
		return []model.ErrorDetail{{Field: "path_filter", Issue: "use '/' as path separator"}}
	}
	if strings.HasPrefix(p, "/") {
		return []model.ErrorDetail{{Field: "path_filter", Issue: "must be a repo-relative prefix, not absolute path"}}
	}
	return nil
}

func (h *AskHandler) handleError(c *gin.Context, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	switch {
	case errors.Is(err, model.ErrRepoNotFound):
		respondError(c, model.CodeRepoNotFound, model.MessageOf(model.CodeRepoNotFound), nil)
	default:
		h.logger.Error("ask failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
	}
}

func errorCodeOf(err error) int {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return model.CodeInternalError
}

func errorMessageOf(err error) string {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return model.MessageOf(model.CodeInternalError)
}
