package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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
	// TODO: GET /api/v1/tasks（§6.7）：?type=&state=&repo_id=&page=&page_size= 过滤分页；
	// items 为 Task 全字段投影（不含 cancel_flag/request_payload，model.Task json tag 已保证）+ pagination。
	respondNotImplemented(c)
}

func (h *TaskHandler) Get(c *gin.Context) {
	// TODO: GET /api/v1/tasks/{task_id}：task_id 先过 ^tsk_[0-9A-HJKMNP-TV-Z]{26}$ 正则；未命中 40401。
	// 对应题目 GET /api/ingest/:id/status 的映射端点（路径以本契约为准，总纲 §2.11）。
	respondNotImplemented(c)
}

func (h *TaskHandler) Cancel(c *gin.Context) {
	// TODO: DELETE /api/v1/tasks/{task_id}（§4.5）：ULID 正则校验；
	// 成功 202 + 当前 task 快照（state 可能尚未变 cancelled）；终态 → 40902；
	// model.ErrTaskNotFound → 40401；model.ErrInvalidTaskState → 40902。
	respondNotImplemented(c)
}
