package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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

func (h *WikiHandler) Generate(c *gin.Context) {
	// TODO: POST /api/v1/wiki/generate（§6.7）：body {"repo_id":"repo_..."} 必填且仓库须 ready（否则 40902）；
	// 202 + dto.TaskSubmittedResponse（type=wiki）；已有 wiki 则覆盖重建；
	// 限流桶 L1 per-IP + L2 wiki_per_hour（总纲 §2.8）。
	respondNotImplemented(c)
}

func (h *WikiHandler) GetWiki(c *gin.Context) {
	// TODO: GET /api/v1/repos/{repo_id}/wiki（§6.7）：
	// {repo_id, task_id, generated_at, toc:[{slug,title,parent_slug,sort_order}], pages:[{slug,title,content_md,sort_order,updated_at}]}；
	// 未生成 → 40403 wiki_not_found。
	respondNotImplemented(c)
}
