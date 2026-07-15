package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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
	// TODO: GET /api/v1/repos（§6.7）：page/page_size 解析（§5.4 默认 1/20、page_size 钳制 1~100）；
	// 响应 dto.PageResult[Repo 摘要]；items 字段：repo_id, repo_url, branch, commit_hash, state, stats, created_at, updated_at。
	respondNotImplemented(c)
}

func (h *RepoHandler) Get(c *gin.Context) {
	// TODO: GET /api/v1/repos/{repo_id}（§6.7）：repo_id 先过 ^repo_[0-9A-HJKMNP-TV-Z]{26}$ 正则（硬约束 #11）；
	// 详情 = 列表字段 + latest_task + wiki_available + chunk_count；未命中 40402。
	respondNotImplemented(c)
}

func (h *RepoHandler) Delete(c *gin.Context) {
	// TODO: DELETE /api/v1/repos/{repo_id}（§6.7、§12.3）：ULID 正则校验；
	// 响应 {repo_id, deleted:{chunks, vectors, wiki_pages, opensearch_docs, local_dir:true}}
	//（关键词索引文档数字段随 OpenSearch 平移更名，语义不变）；任务历史保留（repo_id 置 NULL）。
	respondNotImplemented(c)
}

func (h *RepoHandler) Refresh(c *gin.Context) {
	// TODO: POST /api/v1/repos/{repo_id}/refresh（§6.7）：无 body；202 同 ingest 的 data 结构（type=refresh）；
	// 仓库非 ready / 进行中任务冲突 → 40902；同仓互斥锁 lock:refresh:<repo_id> 持锁失败 → 40902；
	// 限流桶 L1 per-IP + L2 ingest_per_hour。
	respondNotImplemented(c)
}
