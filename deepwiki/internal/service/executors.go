package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/ingest"
	"deepwiki/internal/llm"
	"deepwiki/internal/model"
	"deepwiki/internal/observability"
	"deepwiki/internal/retriever"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

func ptrState(s model.TaskState) *model.TaskState { return &s }

// ---------------- ingest executor ----------------

// IngestExecutor ingest 任务执行器（五阶段 Pipeline，轮次 4b 已实现）。
type IngestExecutor struct {
	tasks      task.TaskStore
	repos      store.RepoStore
	cloner     ingest.Cloner
	embedder   ingest.Embedder
	chunks     store.ChunkStore
	bus        eventbus.EventBus
	cfg        *config.Manager
	onAutoWiki func(ctx context.Context, repoID string)
	logger     *zap.Logger
}

func NewIngestExecutor(tasks task.TaskStore, repos store.RepoStore, cloner ingest.Cloner, embedder ingest.Embedder, chunks store.ChunkStore, bus eventbus.EventBus, cfg *config.Manager, onAutoWiki func(ctx context.Context, repoID string), logger *zap.Logger) *IngestExecutor {
	return &IngestExecutor{tasks: tasks, repos: repos, cloner: cloner, embedder: embedder, chunks: chunks, bus: bus, cfg: cfg, onAutoWiki: onAutoWiki, logger: logger}
}

func (e *IngestExecutor) Type() model.TaskType { return model.TaskTypeIngest }

type ingestTaskPayload struct {
	RepoURL  string                 `json:"repo_url"`
	Branch   string                 `json:"branch"`
	AutoWiki bool                   `json:"auto_wiki"`
	Options  *dto.IngestOptionsDTO  `json:"options"`
}

func (e *IngestExecutor) Execute(ctx context.Context, t *model.Task) error {
	repo, err := e.repos.Get(ctx, t.RepoID)
	if err != nil {
		return err
	}
	var payload ingestTaskPayload
	if len(t.RequestPayload) > 0 {
		_ = json.Unmarshal(t.RequestPayload, &payload)
	}
	if payload.RepoURL == "" {
		payload.RepoURL = repo.RepoURL
		payload.Branch = repo.Branch
	}

	cfg := e.cfg.Get()
	workdir := cfg.Ingest.Workdir
	tmpDir := filepath.Join(workdir, ".tmp", t.TaskID)
	finalDir := filepath.Join(workdir, repo.RepoID)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	options := buildIngestOptions(cfg, payload.Options)
	pc := &ingest.PipelineContext{
		Task:    t,
		Repo:    repo,
		Options: options,
		WorkDir: tmpDir,
	}

	stages := ingest.NewIngestStages(ingest.StageDeps{Cloner: e.cloner, Embedder: e.embedder, Chunks: e.chunks})
	pipeline := ingest.NewPipeline(stages, e.bus, e.logger)
	report := e.progressReporter(t)

	runErr := pipeline.Run(ctx, pc, report)
	if runErr != nil {
		var stageErr *ingest.StageError
		if errors.As(runErr, &stageErr) {
			observability.IncIngest("failure")
			e.logger.Error("ingest stage failed",
				zap.String("task_id", t.TaskID),
				zap.String("stage", string(stageErr.Stage)),
				zap.Error(stageErr.Err),
			)
			e.failTask(ctx, t.TaskID, stageErr.Stage)
			e.markRepoError(ctx, repo.RepoID)
			return nil
		}
		return runErr // ctx 取消或瞬时错误由 Manager 处理
	}

	// 成功：临时目录原子 rename 到正式目录，更新 repo 状态。
	_ = os.RemoveAll(finalDir)
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		return fmt.Errorf("rename workdir: %w", err)
	}

	stats := model.Stats{Files: len(pc.Files), Chunks: len(pc.Chunks)}
	for _, c := range pc.Chunks {
		stats.Tokens += ingest.EstimateTokens(c.Content)
	}
	repo.State = "ready"
	repo.LocalPath = finalDir
	repo.Stats = model.RepoStats(stats)
	repo.UpdatedAt = time.Now().UTC()
	if err := e.repos.Update(ctx, repo); err != nil {
		e.logger.Error("update repo after ingest failed", zap.String("repo_id", repo.RepoID), zap.Error(err))
	}

	if payload.AutoWiki && e.onAutoWiki != nil {
		e.onAutoWiki(ctx, repo.RepoID)
	}
	observability.IncIngest("success")
	observability.AddChunkIndex(len(pc.Chunks))
	return nil
}

func (e *IngestExecutor) progressReporter(t *model.Task) ingest.ProgressReport {
	return func(state model.TaskState, progress model.Progress, stats model.Stats) error {
		return e.tasks.UpdateState(context.Background(), t.TaskID, model.TaskPatch{State: ptrState(state), Progress: &progress, Stats: &stats})
	}
}

func (e *IngestExecutor) failTask(ctx context.Context, taskID string, stage model.TaskState) {
	now := time.Now().UTC()
	failed := model.TaskStateFailed
	_ = e.tasks.UpdateState(ctx, taskID, model.TaskPatch{
		State:      &failed,
		Err:        &model.TaskError{Code: model.CodeTaskInterrupted, Message: model.MessageOf(model.CodeTaskInterrupted), Stage: string(stage)},
		FinishedAt: &now,
	})
}

func (e *IngestExecutor) markRepoError(ctx context.Context, repoID string) {
	repo, err := e.repos.Get(ctx, repoID)
	if err != nil {
		return
	}
	repo.State = "error"
	repo.UpdatedAt = time.Now().UTC()
	_ = e.repos.Update(ctx, repo)
}

func buildIngestOptions(cfg *config.Config, override *dto.IngestOptionsDTO) ingest.IngestOptions {
	opt := ingest.IngestOptions{
		ChunkSize:    cfg.Ingest.ChunkSize,
		ChunkOverlap: cfg.Ingest.ChunkOverlap,
		IncludeExt:   cfg.Ingest.IncludeExt,
		ExcludeDirs:  cfg.Ingest.ExcludeDirs,
	}
	if override == nil {
		return opt
	}
	if override.ChunkSize != nil {
		opt.ChunkSize = *override.ChunkSize
	}
	if override.ChunkOverlap != nil {
		opt.ChunkOverlap = *override.ChunkOverlap
	}
	if override.IncludeExt != nil {
		opt.IncludeExt = override.IncludeExt
	}
	if override.ExcludeDirs != nil {
		opt.ExcludeDirs = override.ExcludeDirs
	}
	return opt
}

// ---------------- refresh executor ----------------

// RefreshExecutor refresh 任务执行器（fetching→diffing→chunking→embedding→persisting）。
// 简化实现：diffing 阶段全量删除旧 chunks 后重建（等效全量刷新，语义安全）。
type RefreshExecutor struct {
	tasks    task.TaskStore
	repos    store.RepoStore
	cloner   ingest.Cloner
	embedder ingest.Embedder
	chunks   store.ChunkStore
	bus      eventbus.EventBus
	cfg      *config.Manager
	logger   *zap.Logger
}

func NewRefreshExecutor(tasks task.TaskStore, repos store.RepoStore, cloner ingest.Cloner, embedder ingest.Embedder, chunks store.ChunkStore, bus eventbus.EventBus, cfg *config.Manager, logger *zap.Logger) *RefreshExecutor {
	return &RefreshExecutor{tasks: tasks, repos: repos, cloner: cloner, embedder: embedder, chunks: chunks, bus: bus, cfg: cfg, logger: logger}
}

func (e *RefreshExecutor) Type() model.TaskType { return model.TaskTypeRefresh }

func (e *RefreshExecutor) Execute(ctx context.Context, t *model.Task) error {
	repo, err := e.repos.Get(ctx, t.RepoID)
	if err != nil {
		return err
	}
	if repo.LocalPath == "" {
		return fmt.Errorf("repo local path empty")
	}
	cfg := e.cfg.Get()
	options := buildIngestOptions(cfg, nil)

	pc := &ingest.PipelineContext{Task: t, Repo: repo, Options: options, WorkDir: repo.LocalPath}

	stages := []ingest.Stage{
		{Name: model.TaskStateFetching, Fn: func(ctx context.Context, pc *ingest.PipelineContext) error {
			newCommit, err := e.cloner.FetchAndReset(ctx, repo.LocalPath, repo.Branch)
			if err != nil {
				return err
			}
			repo.CommitHash = newCommit
			return nil
		}},
		{Name: model.TaskStateDiffing, Fn: func(ctx context.Context, pc *ingest.PipelineContext) error {
			// 真增量 diff：解析当前文件树得 path→hash，与 chunks 表 FileHashes 比对，
			// 只删除（modified ∪ deleted）对应 chunks，只保留变更文件进入后续阶段。
			files, err := ingest.ParseFiles(ctx, repo.LocalPath, options)
			if err != nil {
				return err
			}
			oldHashes, err := e.chunks.FileHashes(ctx, repo.RepoID)
			if err != nil {
				return err
			}
			newHashes := make(map[string]string, len(files))
			for _, f := range files {
				newHashes[f.Path] = f.Hash
			}
			var dirtyPaths []string
			var changedFiles []ingest.SourceFile
			for _, f := range files {
				if old, ok := oldHashes[f.Path]; !ok || old != f.Hash {
					dirtyPaths = append(dirtyPaths, f.Path) // 新增或内容变更
					changedFiles = append(changedFiles, f)
				}
			}
			for p := range oldHashes {
				if _, ok := newHashes[p]; !ok {
					dirtyPaths = append(dirtyPaths, p) // 已删除文件
				}
			}
			if len(dirtyPaths) > 0 {
				if err := e.chunks.DeleteByPaths(ctx, repo.RepoID, dirtyPaths); err != nil {
					return err
				}
			}
			e.logger.Info("refresh diff done",
				zap.String("repo_id", repo.RepoID),
				zap.Int("total_files", len(files)),
				zap.Int("changed", len(changedFiles)),
				zap.Int("dirty_paths", len(dirtyPaths)),
			)
			pc.Files = changedFiles
			return nil
		}},
		{Name: model.TaskStateChunking, Fn: func(ctx context.Context, pc *ingest.PipelineContext) error {
			// 仅切分 diffing 阶段筛出的变更文件。
			chunks, err := ingest.ChunkFiles(ctx, repo.RepoID, pc.Files, options)
			if err != nil {
				return err
			}
			pc.Chunks = chunks
			return nil
		}},
		{Name: model.TaskStateEmbedding, Fn: func(ctx context.Context, pc *ingest.PipelineContext) error {
			if len(pc.Chunks) == 0 {
				return nil
			}
			texts := make([]string, len(pc.Chunks))
			for i, c := range pc.Chunks {
				texts[i] = c.Content
			}
			vectors, err := e.embedder.Embed(ctx, texts)
			if err != nil {
				return err
			}
			modelName := e.embedder.ModelName()
			for i := range pc.Chunks {
				pc.Chunks[i].Vector = vectors[i]
				pc.Chunks[i].EmbeddingModel = modelName
			}
			return nil
		}},
		{Name: model.TaskStatePersisting, Fn: func(ctx context.Context, pc *ingest.PipelineContext) error {
			return e.chunks.InsertBatch(ctx, pc.Chunks)
		}},
	}

	pipeline := ingest.NewPipeline(stages, e.bus, e.logger)
	report := func(state model.TaskState, progress model.Progress, stats model.Stats) error {
		return e.tasks.UpdateState(context.Background(), t.TaskID, model.TaskPatch{State: ptrState(state), Progress: &progress, Stats: &stats})
	}

	runErr := pipeline.Run(ctx, pc, report)
	if runErr != nil {
		var stageErr *ingest.StageError
		if errors.As(runErr, &stageErr) {
			now := time.Now().UTC()
			failed := model.TaskStateFailed
			_ = e.tasks.UpdateState(ctx, t.TaskID, model.TaskPatch{
				State:      &failed,
				Err:        &model.TaskError{Code: model.CodeTaskInterrupted, Message: model.MessageOf(model.CodeTaskInterrupted), Stage: string(stageErr.Stage)},
				FinishedAt: &now,
			})
			return nil
		}
		return runErr
	}

	repo.UpdatedAt = time.Now().UTC()
	_ = e.repos.Update(ctx, repo)
	observability.AddChunkIndex(len(pc.Chunks))
	return nil
}

// ---------------- wiki executor ----------------

// WikiExecutor wiki 任务执行器（outlining→generating→completed）。
type WikiExecutor struct {
	tasks      task.TaskStore
	repos      store.RepoStore
	wikis      store.WikiStore
	chunks     store.ChunkStore
	retrievers map[string]retriever.Retriever
	llm        llm.LLM
	cfg        *config.Manager
	bus        eventbus.EventBus
	logger     *zap.Logger
}

func NewWikiExecutor(tasks task.TaskStore, repos store.RepoStore, wikis store.WikiStore, chunks store.ChunkStore, retrievers map[string]retriever.Retriever, l llm.LLM, cfg *config.Manager, bus eventbus.EventBus, logger *zap.Logger) *WikiExecutor {
	return &WikiExecutor{tasks: tasks, repos: repos, wikis: wikis, chunks: chunks, retrievers: retrievers, llm: l, cfg: cfg, bus: bus, logger: logger}
}

func (e *WikiExecutor) Type() model.TaskType { return model.TaskTypeWiki }

type wikiTOCPlan struct {
	TOC []store.WikiTOCItem `json:"toc"`
}

func (e *WikiExecutor) Execute(ctx context.Context, t *model.Task) error {
	repo, err := e.repos.Get(ctx, t.RepoID)
	if err != nil {
		return err
	}
	cfg := e.cfg.Get()
	r, ok := e.retrievers[cfg.Retriever.Mode]
	if !ok {
		r = e.retrievers["keyword"]
	}
	if r == nil {
		return fmt.Errorf("no retriever available")
	}

	update := func(state model.TaskState, percent int) error {
		progress := model.Progress{Current: percent, Total: 100, Percent: percent}
		if err := e.tasks.UpdateState(ctx, t.TaskID, model.TaskPatch{State: ptrState(state), Progress: &progress}); err != nil {
			return err
		}
		publishStateEvent(ctx, e.bus, e.logger, t, state, progress, model.Stats{})
		return nil
	}

	// outlining：用 LLM 生成 TOC。
	if err := update(model.TaskStateOutlining, 5); err != nil {
		return err
	}
	hits, err := r.Search(ctx, repo.RepoID, "project architecture overview", 20)
	if err != nil {
		e.failTask(ctx, t.TaskID, model.TaskStateOutlining)
		return nil
	}
	toc, err := e.generateTOC(ctx, repo, hits)
	if err != nil {
		e.failTask(ctx, t.TaskID, model.TaskStateOutlining)
		return nil
	}
	if len(toc) == 0 {
		toc = defaultTOC()
	}

	// generating：逐页生成 markdown。断点续跑：已有未完成 wiki（pages<toc 且 TOC slug 一致）
	// 时跳过已生成页——worker 中途死亡重投后只补剩余页，不整仓重来。
	if err := update(model.TaskStateGenerating, 15); err != nil {
		return err
	}

	skip := map[string]bool{}
	pages := make([]store.WikiPage, 0, len(toc))
	if existing, err := e.wikis.Get(ctx, repo.RepoID); err == nil && existing != nil &&
		len(existing.Pages) < len(existing.TOC) && sameTOCSlugs(existing.TOC, toc) {
		for _, p := range existing.Pages {
			skip[p.Slug] = true
			pages = append(pages, p)
		}
		e.logger.Info("wiki resume from checkpoint",
			zap.String("repo_id", repo.RepoID),
			zap.Int("existing_pages", len(existing.Pages)),
			zap.Int("total_pages", len(toc)),
		)
	}

	saveCheckpoint := func() {
		if err := e.wikis.Save(ctx, &store.Wiki{
			RepoID:      repo.RepoID,
			TOC:         toc,
			Pages:       pages,
			TaskID:      t.TaskID,
			GeneratedAt: time.Now().UTC(),
		}); err != nil {
			e.logger.Warn("wiki checkpoint save failed", zap.String("repo_id", repo.RepoID), zap.Error(err))
		}
	}

	donePages := len(pages)
	for _, item := range toc {
		if skip[item.Slug] {
			continue // 已完成页：断点跳过
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pageHits, _ := r.Search(ctx, repo.RepoID, item.Title, 8)
		content, err := e.generatePage(ctx, repo, item, pageHits)
		if err != nil {
			saveCheckpoint() // 失败也先落已完成的页，便于下次续跑
			e.failTask(ctx, t.TaskID, model.TaskStateGenerating)
			return nil
		}
		pages = append(pages, store.WikiPage{
			Slug:      item.Slug,
			Title:     item.Title,
			ContentMD: content,
			SortOrder: item.SortOrder,
			UpdatedAt: time.Now().UTC(),
		})
		donePages++
		saveCheckpoint() // 逐页落库：worker 死亡不丢已完成进度
		percent := 15 + 80*donePages/len(toc)
		if err := update(model.TaskStateGenerating, percent); err != nil {
			return err
		}
	}

	wiki := &store.Wiki{
		RepoID:      repo.RepoID,
		TOC:         toc,
		Pages:       pages,
		TaskID:      t.TaskID,
		GeneratedAt: time.Now().UTC(),
	}
	if err := e.wikis.Save(ctx, wiki); err != nil {
		return err
	}

	completed := model.TaskStateCompleted
	now := time.Now().UTC()
	progress := model.Progress{Current: 100, Total: 100, Percent: 100}
	if err := e.tasks.UpdateState(ctx, t.TaskID, model.TaskPatch{State: &completed, Progress: &progress, FinishedAt: &now}); err != nil {
		return err
	}
	publishStateEvent(ctx, e.bus, e.logger, t, model.TaskStateCompleted, progress, model.Stats{})
	return nil
}

func (e *WikiExecutor) generateTOC(ctx context.Context, repo *model.Repo, hits []model.ChunkHit) ([]store.WikiTOCItem, error) {
	// 目录依据升级为全量文件树（FileHashes 覆盖仓库全部文件路径），比单次检索更完整。
	paths := e.repoFilePaths(ctx, repo.RepoID)
	if len(paths) == 0 {
		seen := map[string]bool{}
		for _, h := range hits { // FileHashes 不可用时回退检索路径
			if !seen[h.Chunk.Path] {
				seen[h.Chunk.Path] = true
				paths = append(paths, h.Chunk.Path)
			}
		}
	}
	prompt := fmt.Sprintf(`你是技术文档架构师。基于仓库 %s（分支 %s）的完整文件树，设计一份 wiki 目录大纲。
要求：
- 按模块/职责划分（如：项目概览、架构设计、核心流程、数据模型、配置与部署、API 参考等，按实际仓库内容取舍）；
- toc 项数量 5~8 个，slug 用英文小写短横线；
- 只输出 JSON：{"toc":[{"slug":"overview","title":"项目概览","parent_slug":"","sort_order":1},...]}，不要输出任何其他文字。
文件树：
%s`, repo.RepoURL, repo.Branch, strings.Join(paths, "\n"))

	resp, err := e.llm.Generate(ctx, model.ChatRequest{
		Model:       e.llm.ModelName(),
		Messages:    []model.ChatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.2,
		MaxTokens:   1500,
	})
	if err != nil {
		return nil, err
	}
	var plan wikiTOCPlan
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &plan); err != nil {
		return nil, err
	}
	return plan.TOC, nil
}

// repoFilePaths 仓库全量文件路径（按目录排序，超 300 条截断保留首尾）。
func (e *WikiExecutor) repoFilePaths(ctx context.Context, repoID string) []string {
	if e.chunks == nil {
		return nil
	}
	hashes, err := e.chunks.FileHashes(ctx, repoID)
	if err != nil || len(hashes) == 0 {
		return nil
	}
	paths := make([]string, 0, len(hashes))
	for p := range hashes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	const maxPaths = 300
	if len(paths) > maxPaths {
		head := paths[:maxPaths/2]
		tail := paths[len(paths)-maxPaths/2:]
		paths = append(append([]string{}, head...), tail...)
	}
	return paths
}

func (e *WikiExecutor) generatePage(ctx context.Context, repo *model.Repo, item store.WikiTOCItem, hits []model.ChunkHit) (string, error) {
	var sb strings.Builder
	for _, h := range hits {
		sb.WriteString(fmt.Sprintf("\n--- %s:%d-%d ---\n%s\n", h.Chunk.Path, h.Chunk.StartLine, h.Chunk.EndLine, h.Chunk.Content))
	}
	mermaidFence := "```" + "mermaid"
	prompt := fmt.Sprintf(`基于以下代码片段，为仓库 %s 撰写 wiki 页面「%s」（markdown 格式）。
要求：
- 结构清晰、包含小标题；引用代码用 [path:start-end] 标注来源，禁止编造文件或行号；
- **必须**插入至少 1 个 mermaid 图（%s 代码块）：架构总览用 flowchart TD，调用时序用 sequenceDiagram，实体关系用 erDiagram，按页面主题选最合适的一种；节点标签用简短中文，禁止在标签中使用引号、括号和分号；
- 图必须基于真实代码结构绘制，放在对应小节正文之后，不要虚构模块。
示例格式：
%s
flowchart TD
  A[用户请求] --> B[路由层]
  B --> C[控制器]
  C --> D[服务层]
%s
代码片段：%s`, repo.RepoURL, item.Title, mermaidFence, mermaidFence, "```", sb.String())

	resp, err := e.llm.Generate(ctx, model.ChatRequest{
		Model:       e.llm.ModelName(),
		Messages:    []model.ChatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.3,
		MaxTokens:   2500,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (e *WikiExecutor) failTask(ctx context.Context, taskID string, stage model.TaskState) {
	now := time.Now().UTC()
	failed := model.TaskStateFailed
	_ = e.tasks.UpdateState(ctx, taskID, model.TaskPatch{
		State:      &failed,
		Err:        &model.TaskError{Code: model.CodeTaskInterrupted, Message: model.MessageOf(model.CodeTaskInterrupted), Stage: string(stage)},
		FinishedAt: &now,
	})
}

// sameTOCSlugs 判断两份 TOC 的 slug 序列是否一致（断点续跑的续接前提；不一致整仓重来）。
func sameTOCSlugs(a, b []store.WikiTOCItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Slug != b[i].Slug {
			return false
		}
	}
	return true
}

func defaultTOC() []store.WikiTOCItem {
	return []store.WikiTOCItem{
		{Slug: "overview", Title: "项目概览", SortOrder: 1},
		{Slug: "architecture", Title: "架构设计", SortOrder: 2},
		{Slug: "getting-started", Title: "快速开始", SortOrder: 3},
	}
}

// publishStateEvent 发布 task.state_changed 事件（失败仅 WARN，任务状态以 Postgres 为准）。
func publishStateEvent(ctx context.Context, bus eventbus.EventBus, logger *zap.Logger, t *model.Task, state model.TaskState, progress model.Progress, stats model.Stats) {
	if bus == nil {
		return
	}
	payload, err := json.Marshal(model.StateChangedPayload{State: state, Progress: progress, Stats: stats})
	if err != nil {
		logger.Warn("marshal state_changed payload failed", zap.Error(err))
		return
	}
	ev := model.Event{
		Type:      model.EventTypeTaskStateChanged,
		RepoID:    t.RepoID,
		TaskID:    t.TaskID,
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(payload),
	}
	if err := bus.Publish(ctx, ev); err != nil {
		logger.Warn("publish state_changed event failed", zap.String("task_id", t.TaskID), zap.Error(err))
	}
}

// extractJSON 从 LLM 输出中提取第一个 JSON 对象（容忍 ```json 包裹）。
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
