package model

import (
	"encoding/json"
	"time"
)

// 事件类型（基线 §6.4，冻结；gap 为断线补发空洞提示）。
const (
	EventTypeTaskStateChanged = "task.state_changed"
	EventTypeTaskProgress     = "task.progress"
	EventTypeWikiCompleted    = "wiki.completed"
	EventTypeGap              = "gap"
)

// StateChangedPayload task.state_changed 事件载荷（字段冻结，结构化字段禁止拼字符串）。
// ingest.Pipeline 与 WikiExecutor 共用；Manager.Submit 的 pending 事件另有 queue_position 扩展字段。
type StateChangedPayload struct {
	State    TaskState `json:"state"`
	Progress Progress  `json:"progress"`
	Stats    Stats     `json:"stats"`
}

// Event 统一事件。Seq 单调递增，是 SSE id / Last-Event-ID 的依据；
// 物理载体为 Redis Streams（events:task:<task_id>，XTRIM MAXLEN ~ 1000）。
// Timestamp 落库/落流均为 UTC + RFC3339（硬约束 #13）。
type Event struct {
	Seq       uint64          `json:"seq"`
	Type      string          `json:"type"`
	RepoID    string          `json:"repo_id"`
	TaskID    string          `json:"task_id"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// EventFilter 订阅过滤；空 = 全部。
// TaskID 用于 SSE/WS 断线回放（Last-Event-ID → XRANGE events:task:<task_id>）。
type EventFilter struct {
	Types  []string
	RepoID string
	TaskID string
}
