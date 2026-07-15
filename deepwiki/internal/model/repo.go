package model

import "time"

// 仓库状态（基线 §12.2 CHECK 约束，冻结）。
const (
	RepoStateIngesting = "ingesting"
	RepoStateReady     = "ready"
	RepoStateError     = "error"
)

// Repo 仓库。LocalPath 不落 API 响应。
// CreatedAt/UpdatedAt 落库为 Postgres timestamptz 列，API 输出 UTC + RFC3339（硬约束 #13）。
type Repo struct {
	RepoID     string    `json:"repo_id"`
	RepoURL    string    `json:"repo_url"`
	Branch     string    `json:"branch"`
	CommitHash string    `json:"commit_hash"`
	LocalPath  string    `json:"-"`
	State      string    `json:"state"` // ingesting|ready|error
	Stats      RepoStats `json:"stats"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RepoStats struct {
	Files  int `json:"files"`
	Chunks int `json:"chunks"`
	Tokens int `json:"tokens"`
}
