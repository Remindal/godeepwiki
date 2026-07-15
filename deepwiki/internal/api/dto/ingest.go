// Package dto API 请求/响应数据结构（纯数据，无行为；与基线 §5、§6 契约一致）。
package dto

// IngestRequest POST /api/v1/ingest 请求体（§6.1）。
type IngestRequest struct {
	RepoURL  string            `json:"repo_url" binding:"required"`
	Branch   string            `json:"branch"`
	AutoWiki bool              `json:"auto_wiki"`
	Options  *IngestOptionsDTO `json:"options"`
}

// IngestOptionsDTO 本次任务的摄取参数覆盖（缺省取 ingest.* 配置）。
type IngestOptionsDTO struct {
	ChunkSize    *int     `json:"chunk_size"`
	ChunkOverlap *int     `json:"chunk_overlap"`
	IncludeExt   []string `json:"include_ext"`
	ExcludeDirs  []string `json:"exclude_dirs"`
}

// TaskSubmittedResponse 建任务类端点（ingest/refresh/wiki）202 响应 data（§6.1、§6.7）。
type TaskSubmittedResponse struct {
	TaskID        string `json:"task_id"`
	RepoID        string `json:"repo_id"`
	Type          string `json:"type"`
	State         string `json:"state"`
	QueuePosition int    `json:"queue_position"`
	CreatedAt     string `json:"created_at"`
}
