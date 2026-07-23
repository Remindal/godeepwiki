package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
)

const testRepoID2 = "repo_01J2X9K7QZ0ABCDEFGHJKMNPQR"

func TestWikiService_GenerateAndGet(t *testing.T) {
	tm := newFakeTaskManager()
	repos := newFakeRepoStore()
	repos.repos[testRepoID2] = &model.Repo{RepoID: testRepoID2, State: "ready"}
	wikis := newFakeWikiStore()

	svc := NewWikiService(tm, repos, wikis, zap.NewNop())
	task, err := svc.Generate(context.Background(), testRepoID2)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if task.Type != model.TaskTypeWiki {
		t.Fatalf("bad task: %+v", task)
	}

	if _, err := svc.GetWiki(context.Background(), testRepoID2); !errors.Is(err, model.ErrWikiNotFound) {
		t.Fatalf("want wiki not found got %v", err)
	}

	wikis.wikis[testRepoID2] = &store.Wiki{RepoID: testRepoID2, TaskID: task.TaskID}
	w, err := svc.GetWiki(context.Background(), testRepoID2)
	if err != nil || w.RepoID != testRepoID2 {
		t.Fatalf("get wiki: %v %+v", err, w)
	}
}

func TestRepoService_ListGetDelete(t *testing.T) {
	repos := newFakeRepoStore()
	repos.repos[testRepoID2] = &model.Repo{RepoID: testRepoID2, RepoURL: "https://github.com/gin-gonic/gin", Branch: "master", State: "ready"}
	chunks := &fakeChunkStore{count: 7}
	wikis := newFakeWikiStore()
	tm := newFakeTaskManager()

	svc := NewRepoService(repos, chunks, &fakeVectorStore{}, wikis, nil, tm, zap.NewNop())

	list, total, err := svc.ListRepos(context.Background(), 1, 20)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %v %d %d", err, total, len(list))
	}

	detail, err := svc.GetRepo(context.Background(), testRepoID2)
	if err != nil || detail.RepoID != testRepoID2 || detail.ChunkCount != 7 {
		t.Fatalf("get: %v %+v", err, detail)
	}

	res, err := svc.DeleteRepo(context.Background(), testRepoID2)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Chunks != 7 || res.Vectors != 7 {
		t.Fatalf("bad delete result: %+v", res)
	}
	if _, err := svc.GetRepo(context.Background(), testRepoID2); !errors.Is(err, model.ErrRepoNotFound) {
		t.Fatalf("repo should be deleted: %v", err)
	}
}
