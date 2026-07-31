package service

import (
	"context"
	"errors"
	"sync"

	"deepwiki/internal/model"
	"deepwiki/internal/queue"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// ---------- fake stores ----------

type fakeRepoStore struct {
	mu    sync.Mutex
	repos map[string]*model.Repo
	byURL map[string]*model.Repo
}

func newFakeRepoStore() *fakeRepoStore {
	return &fakeRepoStore{repos: map[string]*model.Repo{}, byURL: map[string]*model.Repo{}}
}

func (f *fakeRepoStore) Create(ctx context.Context, r *model.Repo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repos[r.RepoID] = r
	f.byURL[r.RepoURL+"|"+r.Branch] = r
	return nil
}
func (f *fakeRepoStore) Get(ctx context.Context, repoID string) (*model.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.repos[repoID]; ok {
		return r, nil
	}
	return nil, model.ErrRepoNotFound
}
func (f *fakeRepoStore) GetByURLBranch(ctx context.Context, url, branch string) (*model.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.byURL[url+"|"+branch]; ok {
		return r, nil
	}
	return nil, model.ErrRepoNotFound
}
func (f *fakeRepoStore) Update(ctx context.Context, r *model.Repo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repos[r.RepoID] = r
	return nil
}
func (f *fakeRepoStore) List(ctx context.Context, page, pageSize int) ([]*model.Repo, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*model.Repo
	for _, r := range f.repos {
		out = append(out, r)
	}
	return out, int64(len(out)), nil
}
func (f *fakeRepoStore) ListRepoIDs(ctx context.Context) ([]string, error) {
	var ids []string
	for id := range f.repos {
		ids = append(ids, id)
	}
	return ids, nil
}
func (f *fakeRepoStore) Delete(ctx context.Context, repoID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.repos, repoID)
	return nil
}

type fakeChunkStore struct {
	count int64
}

func (f *fakeChunkStore) InsertBatch(ctx context.Context, chunks []model.Chunk) error { return nil }
func (f *fakeChunkStore) GetByID(ctx context.Context, chunkID string) (*model.Chunk, error) {
	return &model.Chunk{ChunkID: chunkID}, nil
}
func (f *fakeChunkStore) GetByIDs(ctx context.Context, ids []string) ([]*model.Chunk, error) {
	return nil, nil
}
func (f *fakeChunkStore) DeleteByRepo(ctx context.Context, repoID string) error { return nil }
func (f *fakeChunkStore) DeleteByPaths(ctx context.Context, repoID string, paths []string) error {
	return nil
}
func (f *fakeChunkStore) FileHashes(ctx context.Context, repoID string) (map[string]string, error) {
	return nil, nil
}
func (f *fakeChunkStore) Count(ctx context.Context, repoID string) (int64, error) {
	return f.count, nil
}
func (f *fakeChunkStore) CountByRepo(ctx context.Context, repoID string) (int64, error) {
	return f.count, nil
}

type fakeVectorStore struct{}

func (f *fakeVectorStore) Upsert(ctx context.Context, chunks []model.Chunk) error { return nil }
func (f *fakeVectorStore) Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error) {
	return nil, nil
}
func (f *fakeVectorStore) DeleteByRepo(ctx context.Context, repoID string) error { return nil }

type fakeWikiStore struct {
	wikis map[string]*store.Wiki
}

func newFakeWikiStore() *fakeWikiStore { return &fakeWikiStore{wikis: map[string]*store.Wiki{}} }
func (f *fakeWikiStore) Save(ctx context.Context, w *store.Wiki) error {
	f.wikis[w.RepoID] = w
	return nil
}
func (f *fakeWikiStore) Get(ctx context.Context, repoID string) (*store.Wiki, error) {
	if w, ok := f.wikis[repoID]; ok {
		return w, nil
	}
	return nil, model.ErrWikiNotFound
}
func (f *fakeWikiStore) DeleteByRepo(ctx context.Context, repoID string) error {
	delete(f.wikis, repoID)
	return nil
}

// ---------- fake task manager ----------

type fakeTaskManager struct {
	mu        sync.Mutex
	submitted []*model.Task
	tasks     map[string]*model.Task
	submitErr error
}

func newFakeTaskManager() *fakeTaskManager {
	return &fakeTaskManager{tasks: map[string]*model.Task{}}
}
func (f *fakeTaskManager) Submit(ctx context.Context, t *model.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.submitErr != nil {
		return f.submitErr
	}
	f.submitted = append(f.submitted, t)
	f.tasks[t.TaskID] = t
	return nil
}
func (f *fakeTaskManager) Cancel(ctx context.Context, taskID string) error { return nil }
func (f *fakeTaskManager) Get(ctx context.Context, taskID string) (*model.Task, error) {
	if t, ok := f.tasks[taskID]; ok {
		return t, nil
	}
	return nil, model.ErrTaskNotFound
}
func (f *fakeTaskManager) List(ctx context.Context, filter model.TaskFilter) ([]*model.Task, int64, error) {
	var out []*model.Task
	for _, t := range f.tasks {
		if filter.RepoID != "" && t.RepoID != filter.RepoID {
			continue
		}
		out = append(out, t)
	}
	return out, int64(len(out)), nil
}
func (f *fakeTaskManager) Stats() task.WorkerStats { return task.WorkerStats{} }

// ---------- fake retriever / llm ----------

type fakeRetriever struct {
	mode string
	hits []model.ChunkHit
	err  error
}

func (f *fakeRetriever) Search(ctx context.Context, repoID, query string, topK int) ([]model.ChunkHit, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.hits, nil
}
func (f *fakeRetriever) Mode() string { return f.mode }

type fakeLLM struct {
	resp    model.ChatResponse
	err     error
	stream  []model.StreamChunk
	streamErr error
}

func (f *fakeLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	if f.err != nil {
		return model.ChatResponse{}, f.err
	}
	return f.resp, nil
}
func (f *fakeLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	ch := make(chan model.StreamChunk, len(f.stream))
	for _, c := range f.stream {
		ch <- c
	}
	close(ch)
	return ch, nil
}
func (f *fakeLLM) ProviderName() string { return "fake" }
func (f *fakeLLM) ModelName() string    { return "fake-model" }

var errFakeLLM = errors.New("fake llm down")

// ---------- fake cloner / publisher ----------

type fakeCloner struct {
	head string
	err  error
}

func (f *fakeCloner) Clone(ctx context.Context, url, branch, destDir string) error { return nil }
func (f *fakeCloner) FetchAndReset(ctx context.Context, repoDir, branch string) (string, error) {
	return f.head, f.err
}
func (f *fakeCloner) LsRemote(ctx context.Context, url, branch string) (string, error) {
	return f.head, f.err
}

type fakePublisher struct {
	depth int
	err   error
}

func (f *fakePublisher) Publish(ctx context.Context, msg queue.TaskMessage) error { return f.err }
func (f *fakePublisher) QueueDepth(ctx context.Context) (int, error) {
	return f.depth, nil
}
func (f *fakePublisher) QueueStats(ctx context.Context) (int, int, error) {
	return f.depth, 1, nil
}
func (f *fakePublisher) Close() error { return nil }
