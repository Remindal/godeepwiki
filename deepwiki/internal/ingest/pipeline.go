package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// StageWeights 各阶段占整体进度的百分比（基线 §4.4 冻结）：
// ingest 15/10/10/50/15，refresh 20/10/10/45/15，wiki 10/90。
var StageWeights = map[model.TaskState]int{
	model.TaskStateCloning:    15,
	model.TaskStateParsing:    10,
	model.TaskStateChunking:   10,
	model.TaskStateEmbedding:  50,
	model.TaskStatePersisting: 15,
	model.TaskStateFetching:   20,
	model.TaskStateDiffing:    10,
	model.TaskStateOutlining:  10,
	model.TaskStateGenerating: 90,
}

// StageError 阶段失败包装；Worker 据 Stage 填 task.error.stage。
type StageError struct {
	Stage model.TaskState
	Err   error
}

func (e *StageError) Error() string { return fmt.Sprintf("stage %s: %v", e.Stage, e.Err) }
func (e *StageError) Unwrap() error { return e.Err }

// Embedder  embedding 阶段依赖（结构型接口，由 service 层注入 embed.Embedder 实现）。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ModelName() string
}

// ChunkPersister persisting 阶段依赖（结构型接口，由 service 层注入 store.ChunkStore 实现）。
type ChunkPersister interface {
	InsertBatch(ctx context.Context, chunks []model.Chunk) error
}

// ChunkIndexer OpenSearch 装块依赖（结构型接口，由 service 层注入 *search.Client 实现）。
// 摄取落库成功后即建/装索引，不再依赖下次启动时的对账重建（新部署免重启即可检索）。
type ChunkIndexer interface {
	CreateIndex(ctx context.Context, repoID string) error
	BulkIndex(ctx context.Context, repoID string, chunks []model.Chunk) error
	DeleteByPaths(ctx context.Context, repoID string, paths []string) error
}

// ErrNoIndexableFiles 过滤后无可索引文件（parsing 阶段守卫，防 0 文件仓库误标 ready）。
var ErrNoIndexableFiles = errors.New("no indexable files after filtering")

// StageDeps 六阶段装配依赖。
type StageDeps struct {
	Cloner   Cloner
	Embedder Embedder
	Chunks   ChunkPersister
	Indexer  ChunkIndexer // 可选：nil 时 persisting 只落库不装索引（测试用）
}

// NewIngestStages 装配 ingest 五阶段（pending 与 completed 由 Run 首尾处理）：
// cloning → parsing → chunking → embedding → persisting。
func NewIngestStages(deps StageDeps) []Stage {
	return []Stage{
		{Name: model.TaskStateCloning, Fn: func(ctx context.Context, pc *PipelineContext) error {
			return deps.Cloner.Clone(ctx, pc.Repo.RepoURL, pc.Repo.Branch, pc.WorkDir)
		}},
		{Name: model.TaskStateParsing, Fn: func(ctx context.Context, pc *PipelineContext) error {
			files, err := ParseFiles(ctx, pc.WorkDir, pc.Options)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return ErrNoIndexableFiles
			}
			pc.Files = files
			return nil
		}},
		{Name: model.TaskStateChunking, Fn: func(ctx context.Context, pc *PipelineContext) error {
			chunks, err := ChunkFiles(ctx, pc.Repo.RepoID, pc.Files, pc.Options)
			if err != nil {
				return err
			}
			pc.Chunks = chunks
			return nil
		}},
		{Name: model.TaskStateEmbedding, Fn: func(ctx context.Context, pc *PipelineContext) error {
			// 父子块双层索引：只给子块（ParentChunkID 非空）打向量；父块不嵌向量（仅供上下文）。
			var childIdx []int
			var texts []string
			for i, c := range pc.Chunks {
				if c.ParentChunkID != "" {
					childIdx = append(childIdx, i)
					texts = append(texts, c.Content)
				}
			}
			if len(texts) == 0 {
				return nil
			}
			vectors, err := deps.Embedder.Embed(ctx, texts)
			if err != nil {
				return err
			}
			if len(vectors) != len(texts) {
				return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(texts))
			}
			modelName := deps.Embedder.ModelName()
			for j, i := range childIdx {
				pc.Chunks[i].Vector = vectors[j]
				pc.Chunks[i].EmbeddingModel = modelName
			}
			return nil
		}},
		{Name: model.TaskStatePersisting, Fn: func(ctx context.Context, pc *PipelineContext) error {
			// 顺序约定：Postgres 落库成功后再装 OpenSearch（PG 为唯一事实源）。
			if err := deps.Chunks.InsertBatch(ctx, pc.Chunks); err != nil {
				return err
			}
			if deps.Indexer != nil {
				if err := deps.Indexer.CreateIndex(ctx, pc.Repo.RepoID); err != nil { // 幂等
					return err
				}
				// BulkIndex 内部跳过父块，只装子块（BM25 检索入口）。
				if err := deps.Indexer.BulkIndex(ctx, pc.Repo.RepoID, pc.Chunks); err != nil {
					return err
				}
			}
			return nil
		}},
	}
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

// Run 顺序执行各阶段：每阶段入口检查 ctx 取消，阶段开始实时回调 report 并发布
// task.state_changed 事件；阶段失败返回带 stage 信息的 StageError；全部完成后按
// completed + 100% 收尾。进度落库节流在 Worker 侧实现，本函数只负责实时回调。
func (p *Pipeline) Run(ctx context.Context, pc *PipelineContext, report ProgressReport) error {
	cumulative := 0
	for _, stage := range p.stages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		progress := model.Progress{Current: cumulative, Total: 100, Percent: cumulative}
		stats := snapshotStats(pc)
		if report != nil {
			if err := report(stage.Name, progress, stats); err != nil {
				return err
			}
		}
		p.publishState(ctx, pc, stage.Name, progress, stats)

		if err := stage.Fn(ctx, pc); err != nil {
			return &StageError{Stage: stage.Name, Err: err}
		}
		cumulative += StageWeights[stage.Name]
	}

	progress := model.Progress{Current: 100, Total: 100, Percent: 100}
	stats := snapshotStats(pc)
	if report != nil {
		if err := report(model.TaskStateCompleted, progress, stats); err != nil {
			return err
		}
	}
	p.publishState(ctx, pc, model.TaskStateCompleted, progress, stats)
	return nil
}

// snapshotStats 从 PipelineContext 中间产物汇总 Stats（token 数为切分同一口径的粗估值）。
func snapshotStats(pc *PipelineContext) model.Stats {
	stats := model.Stats{Files: len(pc.Files), Chunks: len(pc.Chunks)}
	for _, c := range pc.Chunks {
		stats.Tokens += EstimateTokens(c.Content)
	}
	return stats
}


// publishState 发布状态事件；发布失败只记 WARN 不中断 Pipeline（任务状态以 Postgres 为准）。
func (p *Pipeline) publishState(ctx context.Context, pc *PipelineContext, state model.TaskState, progress model.Progress, stats model.Stats) {
	if p.bus == nil {
		return
	}
	payload, err := json.Marshal(model.StateChangedPayload{State: state, Progress: progress, Stats: stats})
	if err != nil {
		p.logger.Warn("marshal state_changed payload failed", zap.Error(err))
		return
	}
	ev := model.Event{
		Type:      model.EventTypeTaskStateChanged,
		TaskID:    pc.Task.TaskID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
	if pc.Repo != nil {
		ev.RepoID = pc.Repo.RepoID
	}
	if err := p.bus.Publish(ctx, ev); err != nil {
		p.logger.Warn("publish task.state_changed failed", zap.String("task_id", pc.Task.TaskID), zap.String("state", string(state)), zap.Error(err))
	}
}
