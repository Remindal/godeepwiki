package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/model"
	"deepwiki/internal/retriever"
)

func testConfigManager() *config.Manager {
	base := &config.Config{
		Worker:    config.WorkerConfig{PoolSize: 2, QueueSize: 100},
		Retriever: config.RetrieverConfig{Mode: "keyword", TopK: 5, RRFK: 60},
		LLM:       config.LLMConfig{Provider: "openai", Model: "fake", Temperature: 0.2, MaxTokens: 1000},
		Ingest:    config.IngestConfig{Workdir: "./data/repos", ChunkSize: 800, ChunkOverlap: 100, IncludeExt: []string{".go"}, ExcludeDirs: []string{".git"}},
	}
	return config.NewManager(base, nil, 0, nil, zap.NewNop())
}

func TestAskService_Ask(t *testing.T) {
	repos := newFakeRepoStore()
	repos.repos["repo_test"] = &model.Repo{RepoID: "repo_test", State: "ready"}

	hits := []model.ChunkHit{{
		Chunk: model.Chunk{ChunkID: "chk_1", RepoID: "repo_test", Path: "main.go", StartLine: 1, EndLine: 10, Language: "go", Content: "package main"},
		Score: 0.9,
	}}
	retrievers := map[string]retriever.Retriever{
		"keyword": &fakeRetriever{mode: "keyword", hits: hits},
	}
	llm := &fakeLLM{resp: model.ChatResponse{Content: "这是 main 包 [main.go:1-10]", Usage: model.Usage{PromptTokens: 10, CompletionTokens: 5}}}

	svc := NewAskService(testConfigManager(), repos, retrievers, llm, zap.NewNop())
	resp, err := svc.Ask(context.Background(), dto.AskRequest{RepoID: "repo_test", Question: "这是什么？"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if resp.Answer == "" || len(resp.References) != 1 || resp.References[0].ChunkID != "chk_1" {
		t.Fatalf("bad response: %+v", resp)
	}
	if resp.Mode != "keyword" || resp.Usage.PromptTokens != 10 {
		t.Fatalf("bad mode/usage: %+v", resp)
	}
}

func TestAskService_AskLLMError(t *testing.T) {
	repos := newFakeRepoStore()
	repos.repos["repo_test"] = &model.Repo{RepoID: "repo_test", State: "ready"}
	retrievers := map[string]retriever.Retriever{"keyword": &fakeRetriever{mode: "keyword"}}
	llm := &fakeLLM{err: errFakeLLM}

	svc := NewAskService(testConfigManager(), repos, retrievers, llm, zap.NewNop())
	_, err := svc.Ask(context.Background(), dto.AskRequest{RepoID: "repo_test", Question: "q"})
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != model.CodeLLMUnavailable {
		t.Fatalf("want 50201 got %v", err)
	}
}

func TestAskService_AskStream(t *testing.T) {
	repos := newFakeRepoStore()
	repos.repos["repo_test"] = &model.Repo{RepoID: "repo_test", State: "ready"}
	hits := []model.ChunkHit{{Chunk: model.Chunk{ChunkID: "chk_1", Path: "a.go", Content: "x"}}}
	retrievers := map[string]retriever.Retriever{"keyword": &fakeRetriever{mode: "keyword", hits: hits}}
	llm := &fakeLLM{stream: []model.StreamChunk{
		{Delta: "你好"},
		{Delta: "世界"},
		{Usage: &model.Usage{PromptTokens: 3, CompletionTokens: 2}},
	}}

	svc := NewAskService(testConfigManager(), repos, retrievers, llm, zap.NewNop())
	var events []string
	err := svc.AskStream(context.Background(), dto.AskRequest{RepoID: "repo_test", Question: "q"}, func(event string, payload any) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ask stream: %v", err)
	}
	if len(events) != 4 || events[0] != "references" || events[1] != "token" || events[2] != "token" || events[3] != "done" {
		t.Fatalf("bad events: %v", events)
	}
}
