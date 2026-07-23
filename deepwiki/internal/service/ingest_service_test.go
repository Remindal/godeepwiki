package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/model"
)

const testRepoID = "repo_01J2X9K7QZ0ABCDEFGHJKMNPQR"

func TestIngestService_SubmitIngest(t *testing.T) {
	tm := newFakeTaskManager()
	repos := newFakeRepoStore()
	cloner := &fakeCloner{head: "abc123"}
	pub := &fakePublisher{depth: 0}

	svc := NewIngestService(tm, repos, cloner, pub, testConfigManager(), zap.NewNop())
	req := dto.IngestRequest{RepoURL: "https://github.com/gin-gonic/gin", Branch: "master"}
	task, repo, err := svc.SubmitIngest(context.Background(), req)
	if err != nil {
		t.Fatalf("submit ingest: %v", err)
	}
	if task.Type != model.TaskTypeIngest || task.State != model.TaskStatePending {
		t.Fatalf("bad task: %+v", task)
	}
	if repo.State != "ingesting" || repo.RepoURL != req.RepoURL {
		t.Fatalf("bad repo: %+v", repo)
	}
	if len(tm.submitted) != 1 {
		t.Fatalf("task not submitted: %d", len(tm.submitted))
	}
}

func TestIngestService_SubmitIngestConflict(t *testing.T) {
	tm := newFakeTaskManager()
	repos := newFakeRepoStore()
	existing := &model.Repo{RepoID: testRepoID, RepoURL: "https://github.com/gin-gonic/gin", Branch: "master", CommitHash: "abc123"}
	repos.repos[testRepoID] = existing
	repos.byURL["https://github.com/gin-gonic/gin|master"] = existing
	cloner := &fakeCloner{head: "abc123"}
	pub := &fakePublisher{depth: 0}

	svc := NewIngestService(tm, repos, cloner, pub, testConfigManager(), zap.NewNop())
	_, _, err := svc.SubmitIngest(context.Background(), dto.IngestRequest{RepoURL: "https://github.com/gin-gonic/gin", Branch: "master"})
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != model.CodeRepoAlreadyExists {
		t.Fatalf("want 40901 got %v", err)
	}
	if apiErr.Details[0].ExistingRepoID != testRepoID {
		t.Fatalf("missing existing_repo_id: %+v", apiErr.Details)
	}
}

func TestIngestService_SubmitIngestQueueFull(t *testing.T) {
	svc := NewIngestService(newFakeTaskManager(), newFakeRepoStore(), &fakeCloner{}, &fakePublisher{depth: 100}, testConfigManager(), zap.NewNop())
	_, _, err := svc.SubmitIngest(context.Background(), dto.IngestRequest{RepoURL: "https://github.com/gin-gonic/gin"})
	if !errors.Is(err, model.ErrQueueFull) {
		t.Fatalf("want ErrQueueFull got %v", err)
	}
}

func TestIngestService_SubmitRefresh(t *testing.T) {
	tm := newFakeTaskManager()
	repos := newFakeRepoStore()
	repos.repos["repo_refresh"] = &model.Repo{RepoID: "repo_refresh", RepoURL: "https://github.com/gin-gonic/gin", Branch: "master", State: "ready"}

	svc := NewIngestService(tm, repos, &fakeCloner{}, &fakePublisher{}, testConfigManager(), zap.NewNop())
	if _, err := svc.SubmitRefresh(context.Background(), "repo_refresh"); err == nil {
		t.Fatal("want invalid repo_id error")
	}
	repos.repos[testRepoID] = &model.Repo{RepoID: testRepoID, RepoURL: "https://github.com/gin-gonic/gin", Branch: "master", State: "ready"}
	task, err := svc.SubmitRefresh(context.Background(), testRepoID)
	if err != nil {
		t.Fatalf("submit refresh: %v", err)
	}
	if task.Type != model.TaskTypeRefresh {
		t.Fatalf("bad task type: %+v", task)
	}
}
