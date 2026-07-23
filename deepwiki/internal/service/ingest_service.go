// Package service 业务编排层：只依赖领域接口，不依赖任何 provider SDK 具体类型（基线 §2.2，硬约束 #17）。
package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"regexp"
	"time"

	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/ingest"
	"deepwiki/internal/model"
	"deepwiki/internal/queue"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

var repoIDRegex = regexp.MustCompile(`^repo_[0-9A-HJKMNP-TV-Z]{26}$`)

// IngestService 摄取与刷新编排。
type IngestService struct {
	tm        task.TaskManager
	repos     store.RepoStore
	cloner    ingest.Cloner
	publisher queue.Publisher // RabbitMQ 背压预检（QueueDepth），避免白做 LsRemote
	cfg       *config.Manager
	logger    *zap.Logger
}

func NewIngestService(tm task.TaskManager, repos store.RepoStore, cloner ingest.Cloner, publisher queue.Publisher, cfg *config.Manager, logger *zap.Logger) *IngestService {
	return &IngestService{tm: tm, repos: repos, cloner: cloner, publisher: publisher, cfg: cfg, logger: logger}
}

func newID(prefix string) string {
	return prefix + ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(rand.Reader, 0)).String()
}

// SubmitIngest POST /api/v1/ingest 的业务实现（§6.1）。
func (s *IngestService) SubmitIngest(ctx context.Context, req dto.IngestRequest) (*model.Task, *model.Repo, error) {
	cfg := s.cfg.Get()

	// ① RabbitMQ 背压预检：队列满直接 42902，避免白做 LsRemote。
	if s.publisher != nil {
		depth, err := s.publisher.QueueDepth(ctx)
		if err != nil {
			s.logger.Warn("queue depth precheck failed", zap.Error(err))
		} else if depth >= cfg.Worker.QueueSize {
			return nil, nil, model.ErrQueueFull
		}
	}

	// ② 远端 HEAD commit（失败放行并记 WARN，不阻断提交）。
	commitHash := ""
	if s.cloner != nil {
		if h, err := s.cloner.LsRemote(ctx, req.RepoURL, req.Branch); err != nil {
			s.logger.Warn("git ls-remote failed", zap.String("repo_url", req.RepoURL), zap.String("branch", req.Branch), zap.Error(err))
		} else {
			commitHash = h
		}
	}

	// ③ 幂等判断：同 repo_url+branch 已存在时按 commit 是否变化区分提示。
	existing, err := s.repos.GetByURLBranch(ctx, req.RepoURL, req.Branch)
	if err == nil && existing != nil {
		if commitHash != "" && existing.CommitHash == commitHash {
			apiErr := model.NewAPIError(model.CodeRepoAlreadyExists, model.MessageOf(model.CodeRepoAlreadyExists))
			apiErr.Details = []model.ErrorDetail{{Field: "repo_url", Issue: "repository already ingested", ExistingRepoID: existing.RepoID}}
			return nil, nil, apiErr
		}
		apiErr := model.NewAPIError(model.CodeRepoAlreadyExists, model.MessageOf(model.CodeRepoAlreadyExists))
		apiErr.Details = []model.ErrorDetail{{Field: "repo_url", Issue: "use_refresh", ExistingRepoID: existing.RepoID}}
		return nil, nil, apiErr
	}
	if err != nil && err != model.ErrRepoNotFound {
		return nil, nil, err
	}

	// ④ 预创建 repo（state=ingesting）。
	repo := &model.Repo{
		RepoID:     newID("repo_"),
		RepoURL:    req.RepoURL,
		Branch:     req.Branch,
		CommitHash: commitHash,
		State:      "ingesting",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := s.repos.Create(ctx, repo); err != nil {
		return nil, nil, err
	}

	// ⑤ 原始请求快照作为 RequestPayload。
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	t := &model.Task{
		TaskID:         newID("tsk_"),
		Type:           model.TaskTypeIngest,
		RepoID:         repo.RepoID,
		State:          model.TaskStatePending,
		RequestPayload: payload,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.tm.Submit(ctx, t); err != nil {
		// 提交失败回滚 repo，避免残留 ingesting 状态。
		if delErr := s.repos.Delete(ctx, repo.RepoID); delErr != nil {
			s.logger.Error("rollback repo failed", zap.String("repo_id", repo.RepoID), zap.Error(delErr))
		}
		return nil, nil, err
	}

	return t, repo, nil
}

// SubmitRefresh POST /api/v1/repos/{repo_id}/refresh 的业务实现（§4.7、§6.7）。
func (s *IngestService) SubmitRefresh(ctx context.Context, repoID string) (*model.Task, error) {
	if !repoIDRegex.MatchString(repoID) {
		return nil, model.NewAPIError(model.CodeInvalidParam, "invalid repo_id format")
	}

	repo, err := s.repos.Get(ctx, repoID)
	if err != nil {
		if err == model.ErrRepoNotFound {
			return nil, model.ErrRepoNotFound
		}
		return nil, err
	}
	if repo.State != "ready" {
		return nil, model.NewAPIError(model.CodeInvalidTaskState, "repo is not ready")
	}

	// 进行中任务冲突检查（ingest/refresh/wiki 任一非终态即拒绝）。
	tasks, _, err := s.tm.List(ctx, model.TaskFilter{RepoID: repoID, Page: 1, PageSize: 100})
	if err == nil {
		for _, t := range tasks {
			if !t.State.IsTerminal() {
				return nil, model.NewAPIError(model.CodeInvalidTaskState, "repo has running task")
			}
		}
	}

	payload, err := json.Marshal(map[string]any{
		"repo_id":  repo.RepoID,
		"repo_url": repo.RepoURL,
		"branch":   repo.Branch,
	})
	if err != nil {
		return nil, err
	}
	t := &model.Task{
		TaskID:         newID("tsk_"),
		Type:           model.TaskTypeRefresh,
		RepoID:         repo.RepoID,
		State:          model.TaskStatePending,
		RequestPayload: payload,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.tm.Submit(ctx, t); err != nil {
		if err == model.ErrRepoAlreadyExists { // Manager refresh 锁冲突 → 40902
			return nil, model.NewAPIError(model.CodeInvalidTaskState, "refresh already in progress")
		}
		return nil, err
	}
	return t, nil
}
