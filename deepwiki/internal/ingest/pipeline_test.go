package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// fakeCloner 在 destDir 写入两个源文件，模拟克隆产物。
type fakeCloner struct{}

func (fakeCloner) Clone(ctx context.Context, url, branch, destDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destDir, "README.md"), []byte("# Demo\n\nhello\n"), 0644)
}

func (fakeCloner) FetchAndReset(ctx context.Context, repoDir, branch string) (string, error) {
	return "", errors.New("not implemented")
}

func (fakeCloner) LsRemote(ctx context.Context, url, branch string) (string, error) {
	return "", errors.New("not implemented")
}

type fakeEmbedder struct{ model string }

func (f fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vectors := make([][]float32, len(texts))
	for i := range vectors {
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

func (f fakeEmbedder) ModelName() string { return f.model }

type fakePersister struct {
	chunks []model.Chunk
}

func (f *fakePersister) InsertBatch(ctx context.Context, chunks []model.Chunk) error {
	f.chunks = append(f.chunks, chunks...)
	return nil
}

type fakeBus struct {
	events []model.Event
}

func (f *fakeBus) Publish(ctx context.Context, ev model.Event) error {
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeBus) Subscribe(filter model.EventFilter) (<-chan model.Event, func()) {
	ch := make(chan model.Event)
	return ch, func() {}
}

type reportCall struct {
	state   model.TaskState
	percent int
	stats   model.Stats
}

func newTestPipelineContext(t *testing.T) *PipelineContext {
	t.Helper()
	return &PipelineContext{
		Task: &model.Task{TaskID: "tsk_test", Type: model.TaskTypeIngest, RepoID: "repo_test"},
		Repo: &model.Repo{RepoID: "repo_test", RepoURL: "https://example.com/repo.git", Branch: "main"},
		Options: IngestOptions{
			ChunkSize:    500,
			ChunkOverlap: 100,
			IncludeExt:   []string{".go", ".md"},
		},
		WorkDir: filepath.Join(t.TempDir(), "work"),
	}
}

func TestPipelineRunStages(t *testing.T) {
	persister := &fakePersister{}
	bus := &fakeBus{}
	stages := NewIngestStages(StageDeps{
		Cloner:   fakeCloner{},
		Embedder: fakeEmbedder{model: "fake-embed-v1"},
		Chunks:   persister,
	})
	p := NewPipeline(stages, bus, zap.NewNop())

	pc := newTestPipelineContext(t)
	var calls []reportCall
	report := func(state model.TaskState, progress model.Progress, stats model.Stats) error {
		calls = append(calls, reportCall{state: state, percent: progress.Percent, stats: stats})
		return nil
	}

	if err := p.Run(context.Background(), pc, report); err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantStates := []model.TaskState{
		model.TaskStateCloning, model.TaskStateParsing, model.TaskStateChunking,
		model.TaskStateEmbedding, model.TaskStatePersisting, model.TaskStateCompleted,
	}
	wantPercents := []int{0, 15, 25, 35, 85, 100}
	if len(calls) != len(wantStates) {
		t.Fatalf("got %d report calls, want %d", len(calls), len(wantStates))
	}
	for i, c := range calls {
		if c.state != wantStates[i] || c.percent != wantPercents[i] {
			t.Fatalf("call %d = (%s, %d%%), want (%s, %d%%)", i, c.state, c.percent, wantStates[i], wantPercents[i])
		}
	}
	if calls[2].stats.Files != 2 {
		t.Fatalf("chunking-stage stats.Files = %d, want 2", calls[2].stats.Files)
	}
	final := calls[len(calls)-1]
	if final.stats.Files != 2 || final.stats.Chunks != len(pc.Chunks) || final.stats.Tokens <= 0 {
		t.Fatalf("final stats = %+v", final.stats)
	}

	if len(bus.events) != 6 {
		t.Fatalf("got %d events, want 6", len(bus.events))
	}
	for i, ev := range bus.events {
		if ev.Type != model.EventTypeTaskStateChanged || ev.TaskID != "tsk_test" || ev.RepoID != "repo_test" {
			t.Fatalf("event %d malformed: %+v", i, ev)
		}
		var payload struct {
			State model.TaskState `json:"state"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("event %d payload: %v", i, err)
		}
		if payload.State != wantStates[i] {
			t.Fatalf("event %d state = %s, want %s", i, payload.State, wantStates[i])
		}
	}

	if len(persister.chunks) == 0 || len(persister.chunks) != len(pc.Chunks) {
		t.Fatalf("persisted %d chunks, pc has %d", len(persister.chunks), len(pc.Chunks))
	}
	for _, c := range persister.chunks {
		if len(c.Vector) != 3 || c.EmbeddingModel != "fake-embed-v1" {
			t.Fatalf("chunk %s missing vector/model: %+v", c.ChunkID, c)
		}
		if c.StartLine < 1 || c.StartLine > c.EndLine || c.Path == "" {
			t.Fatalf("chunk %s invalid span/path: %+v", c.ChunkID, c)
		}
	}
}

func TestPipelineCancelBeforeRun(t *testing.T) {
	p := NewPipeline(NewIngestStages(StageDeps{Cloner: fakeCloner{}, Embedder: fakeEmbedder{}, Chunks: &fakePersister{}}), nil, zap.NewNop())
	pc := newTestPipelineContext(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Run(ctx, pc, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestPipelineCancelBetweenStages(t *testing.T) {
	p := NewPipeline(NewIngestStages(StageDeps{Cloner: fakeCloner{}, Embedder: fakeEmbedder{}, Chunks: &fakePersister{}}), nil, zap.NewNop())
	pc := newTestPipelineContext(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	report := func(state model.TaskState, progress model.Progress, stats model.Stats) error {
		if state == model.TaskStateEmbedding {
			cancel()
		}
		return nil
	}
	if err := p.Run(ctx, pc, report); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled at next checkpoint", err)
	}
}

func TestPipelineStageError(t *testing.T) {
	failEmbed := StageDeps{
		Cloner:   fakeCloner{},
		Embedder: failingEmbedder{},
		Chunks:   &fakePersister{},
	}
	p := NewPipeline(NewIngestStages(failEmbed), nil, zap.NewNop())
	pc := newTestPipelineContext(t)
	err := p.Run(context.Background(), pc, nil)
	var stageErr *StageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("err = %v, want StageError", err)
	}
	if stageErr.Stage != model.TaskStateEmbedding {
		t.Fatalf("stage = %s, want embedding", stageErr.Stage)
	}
}

type failingEmbedder struct{}

func (failingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("embed api down")
}

func (failingEmbedder) ModelName() string { return "failing" }
