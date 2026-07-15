package ingest

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/eventbus"
	"deepwiki/internal/model"
)

// StageFunc Pipeline 阶段函数；每阶段入口与循环内必须 select ctx.Done()（反 AI 错误 #4）。
type StageFunc func(ctx context.Context, pc *PipelineContext) error

// PipelineContext 阶段间上下文（基线 §7，冻结字段）。
// WorkDir 为本任务临时工作目录 ./data/repos/.tmp/<task_id>；
// Files / Chunks 为阶段间中间产物（内存传递，落库以 Persist 阶段为准）。
type PipelineContext struct {
	Task    *model.Task
	Repo    *model.Repo
	Options IngestOptions // 本次任务生效的摄取参数（请求 options 覆盖配置后的结果）
	WorkDir string
	Files   []SourceFile
	Chunks  []model.Chunk
}

// IngestOptions 本次任务生效的摄取参数（基线 §7）。
type IngestOptions struct {
	ChunkSize    int
	ChunkOverlap int
	IncludeExt   []string
	ExcludeDirs  []string
}

// SourceFile 解析阶段的文件中间产物（基线 §7）。
type SourceFile struct {
	Path     string
	Language string
	Content  string
	Hash     string // sha256(content)[:16]
}

// Stage 命名阶段（Name 为阶段对应的 TaskState，用于状态推进与事件发布）。
type Stage struct {
	Name model.TaskState
	Fn   StageFunc
}

// Pipeline 顺序阶段执行器。
type Pipeline struct {
	stages []Stage
	bus    eventbus.EventBus
	logger *zap.Logger
}

func NewPipeline(stages []Stage, bus eventbus.EventBus, logger *zap.Logger) *Pipeline {
	return &Pipeline{stages: stages, bus: bus, logger: logger}
}

// report 进度回调：由调用方（Worker）实现 UpdateState + 落库节流（每 500ms 或每推进 5% 一次，基线 §4.4）。
type ProgressReport func(state model.TaskState, progress model.Progress, stats model.Stats) error

func (p *Pipeline) Run(ctx context.Context, pc *PipelineContext, report ProgressReport) error {
	// TODO: 顺序执行 p.stages，要求：
	// ① 每阶段入口必须 select ctx.Done()，取消时返回 ctx.Err()（反 AI 错误 #4，Worker 据此落 cancelled）；
	// ② 每阶段开始调用 report(stage.Name, ...) 并向 p.bus.Publish task.state_changed 事件（结构化字段，禁止拼字符串）；
	// ③ 阶段错误原样返回（错误须带 stage 信息，供 Worker 落 failed 时填 error.stage）；
	// ④ 进度落库节流在 Worker 侧实现，本函数只负责实时回调。
	panic("TODO: Pipeline.Run not implemented")
}
