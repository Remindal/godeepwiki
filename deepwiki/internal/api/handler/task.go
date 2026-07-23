package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/model"
	"deepwiki/internal/task"
)

// TaskHandler /api/v1/tasks 统一任务端点（ingest/refresh/wiki 共用，type 字段区分）。
type TaskHandler struct {
	tm     task.TaskManager
	logger *zap.Logger
}

func NewTaskHandler(tm task.TaskManager, logger *zap.Logger) *TaskHandler {
	return &TaskHandler{tm: tm, logger: logger}
}

func (h *TaskHandler) List(c *gin.Context) {
	var filter model.TaskFilter
	if t := c.Query("type"); t != "" {
		tt := model.TaskType(t)
		filter.Type = &tt
	}
	if s := c.Query("state"); s != "" {
		ss := model.TaskState(s)
		filter.State = &ss
	}
	filter.RepoID = c.Query("repo_id")
	filter.Page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	filter.PageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))

	tasks, total, err := h.tm.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list tasks failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
		return
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)
	respondOK(c, dto.PageResult[*model.Task]{
		Items: tasks,
		Pagination: dto.Pagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *TaskHandler) Get(c *gin.Context) {
	taskID := c.Param("task_id")
	if !validTaskID(taskID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidTaskIDDetail("task_id"))
		return
	}
	t, err := h.tm.Get(c.Request.Context(), taskID)
	if err != nil {
		h.handleTaskError(c, err)
		return
	}
	respondOK(c, t)
}

func (h *TaskHandler) Cancel(c *gin.Context) {
	taskID := c.Param("task_id")
	if !validTaskID(taskID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidTaskIDDetail("task_id"))
		return
	}
	if err := h.tm.Cancel(c.Request.Context(), taskID); err != nil {
		h.handleTaskError(c, err)
		return
	}
	t, err := h.tm.Get(c.Request.Context(), taskID)
	if err != nil {
		h.handleTaskError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, model.Envelope{
		Code:      0,
		Message:   "ok",
		Data:      t,
		RequestID: requestID(c),
	})
}

func (h *TaskHandler) handleTaskError(c *gin.Context, err error) {
	var apiErr *model.APIError
	if errors.As(err, &apiErr) {
		respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	switch {
	case errors.Is(err, model.ErrTaskNotFound):
		respondError(c, model.CodeTaskNotFound, model.MessageOf(model.CodeTaskNotFound), nil)
	case errors.Is(err, model.ErrInvalidTaskState):
		respondError(c, model.CodeInvalidTaskState, model.MessageOf(model.CodeInvalidTaskState), nil)
	default:
		h.logger.Error("task operation failed", zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
	}
}
