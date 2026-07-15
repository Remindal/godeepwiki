// Package model 领域类型与错误码。最内层，禁止 import 任何 internal 包与 provider SDK。
package model

import (
	"encoding/json"
	"time"
)

// TaskType 任务类型（基线 §4.2，冻结）。
type TaskType string

const (
	TaskTypeIngest  TaskType = "ingest"
	TaskTypeRefresh TaskType = "refresh"
	TaskTypeWiki    TaskType = "wiki"
)

// TaskState 任务状态（基线 §4.3，冻结）。任务生命周期只由 state 一个字段表达，
// 禁止另设 status/phase 等二义字段。
type TaskState string

const (
	TaskStatePending    TaskState = "pending"
	TaskStateCloning    TaskState = "cloning"
	TaskStateParsing    TaskState = "parsing"
	TaskStateChunking   TaskState = "chunking"
	TaskStateEmbedding  TaskState = "embedding"
	TaskStatePersisting TaskState = "persisting"
	TaskStateOutlining  TaskState = "outlining"  // wiki 专用
	TaskStateGenerating TaskState = "generating" // wiki 专用
	TaskStateFetching   TaskState = "fetching"   // refresh 专用
	TaskStateDiffing    TaskState = "diffing"    // refresh 专用
	TaskStateCompleted  TaskState = "completed"
	TaskStateFailed     TaskState = "failed"
	TaskStateCancelled  TaskState = "cancelled"
)

// IsTerminal 终态：completed / failed / cancelled。进入终态必须写 finished_at 且不可再转移。
func (s TaskState) IsTerminal() bool {
	return s == TaskStateCompleted || s == TaskStateFailed || s == TaskStateCancelled
}

// validTransitions 合法状态转移表（基线 §4.3 三类状态机的并集，冻结）。
var validTransitions = map[TaskState][]TaskState{
	TaskStatePending:    {TaskStateCloning, TaskStateOutlining, TaskStateFetching, TaskStateCancelled},
	TaskStateCloning:    {TaskStateParsing, TaskStateCancelled, TaskStateFailed},
	TaskStateParsing:    {TaskStateChunking, TaskStateCancelled, TaskStateFailed},
	TaskStateChunking:   {TaskStateEmbedding, TaskStateCancelled, TaskStateFailed},
	TaskStateEmbedding:  {TaskStatePersisting, TaskStateCancelled, TaskStateFailed},
	TaskStatePersisting: {TaskStateCompleted, TaskStateCancelled, TaskStateFailed},
	TaskStateOutlining:  {TaskStateGenerating, TaskStateCancelled, TaskStateFailed},
	TaskStateGenerating: {TaskStateCompleted, TaskStateCancelled, TaskStateFailed},
	TaskStateFetching:   {TaskStateDiffing, TaskStateCancelled, TaskStateFailed},
	TaskStateDiffing:    {TaskStateChunking, TaskStateCancelled, TaskStateFailed},
	TaskStateCompleted:  {},
	TaskStateFailed:     {},
	TaskStateCancelled:  {},
}

// CanTransition 状态机转移校验；TaskStore.UpdateState 必须调用它，非法转移返回 40902。
func CanTransition(from, to TaskState) bool {
	for _, s := range validTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Progress 阶段内进度；Total=0 表示阶段总量未知（不确定进度条）。
type Progress struct {
	Current int `json:"current"`
	Total   int `json:"total"`
	Percent int `json:"percent"` // 0~100，阶段内
}

// Stats 随阶段累计；无数据时全 0。
type Stats struct {
	Files  int `json:"files"`
	Chunks int `json:"chunks"`
	Tokens int `json:"tokens"`
}

// TaskError 失败时写入；正常/运行中为 null。
type TaskError struct {
	Code    int    `json:"code"`    // 复用统一错误码空间（如 50004）
	Message string `json:"message"` // 脱敏后的面向用户描述
	Stage   string `json:"stage"`   // 失败时所处的 state
}

// Task 任务全字段（基线 §4.2）。CancelFlag / RequestPayload 不落 API 响应。
// 时间字段（CreatedAt/StartedAt/FinishedAt）落库为 Postgres timestamptz 列，
// API 输出统一 UTC + RFC3339（硬约束 #13）。
type Task struct {
	TaskID         string          `json:"task_id"`
	Type           TaskType        `json:"type"`
	RepoID         string          `json:"repo_id"`
	State          TaskState       `json:"state"`
	Progress       Progress        `json:"progress"`
	Stats          Stats           `json:"stats"`
	Err            *TaskError      `json:"error"`
	QueuePosition  int             `json:"queue_position"`
	CancelFlag     bool            `json:"-"`
	RequestPayload json.RawMessage `json:"-"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
}

// TaskFilter 任务列表过滤（§6.7：type/state/repo_id + 分页）。
type TaskFilter struct {
	Type     *TaskType
	State    *TaskState
	RepoID   string
	Page     int
	PageSize int
}

// TaskPatch 增量更新补丁；指针字段为 nil 表示不更新。Err 与 ClearErr 互斥。
type TaskPatch struct {
	State         *TaskState
	Progress      *Progress
	Stats         *Stats
	Err           *TaskError
	ClearErr      bool
	QueuePosition *int
	StartedAt     *time.Time
	FinishedAt    *time.Time
}
