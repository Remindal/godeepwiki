// Package task 统一任务系统：ingest / refresh / wiki 三类任务共用同一套
// 任务模型、状态机、Worker Pool、查询/取消端点（基线 §4）。
// 存储实现为 Postgres tasks 表（总纲 R1：pgx/v5 + pgxpool，参数化 $n 占位，硬约束 #11）。
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// TaskStore 任务持久化（基线 §7，冻结签名；硬约束 #3：Postgres tasks 表为任务状态唯一来源，
// 禁止用内存 map 作为唯一存储；内存中只允许保存运行中任务的 context.Context 与 cancel 函数）。
type TaskStore interface {
	Create(ctx context.Context, t *model.Task) error
	Get(ctx context.Context, taskID string) (*model.Task, error)
	// UpdateState 内置状态机转移校验（model.CanTransition）；非法转移返回 40902。
	UpdateState(ctx context.Context, taskID string, patch model.TaskPatch) error
	List(ctx context.Context, f model.TaskFilter) (tasks []*model.Task, total int64, err error)
	SetCancelFlag(ctx context.Context, taskID string) error
	// FindInterrupted 启动恢复用：查出全部非终态任务（§4.6，Reconciler 数据源）。
	FindInterrupted(ctx context.Context) ([]*model.Task, error)
}

// postgresTaskStore tasks 表 Postgres 访问实现（pgxpool 连接池；$n 占位参数化，硬约束 #11）。
type postgresTaskStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewTaskStore 返回具体类型指针：同时满足 TaskStore（冻结接口）与 queue.RecoveryStore
//（ResetStaleRunning 为启动恢复扩展方法，不进入冻结接口）。
func NewTaskStore(pool *pgxpool.Pool, logger *zap.Logger) *postgresTaskStore {
	return &postgresTaskStore{pool: pool, logger: logger}
}

var _ TaskStore = (*postgresTaskStore)(nil)

var taskColumns = strings.Join([]string{
	"task_id", "type", "repo_id", "state",
	"progress_json", "stats_json", "error_json",
	"queue_position", "cancel_flag", "request_payload_json",
	"created_at", "started_at", "finished_at",
}, ", ")

func (s *postgresTaskStore) Create(ctx context.Context, t *model.Task) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}

	progressJSON, err := json.Marshal(t.Progress)
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}
	statsJSON, err := json.Marshal(t.Stats)
	if err != nil {
		return fmt.Errorf("marshal stats: %w", err)
	}
	var errJSON []byte
	if t.Err != nil {
		errJSON, err = json.Marshal(t.Err)
		if err != nil {
			return fmt.Errorf("marshal error: %w", err)
		}
	}
	cancelFlag := 0
	if t.CancelFlag {
		cancelFlag = 1
	}
	var repoID *string
	if t.RepoID != "" {
		repoID = &t.RepoID
	}

	sql := `INSERT INTO tasks (` + taskColumns + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

	_, err = s.pool.Exec(ctx, sql,
		t.TaskID, string(t.Type), repoID, string(t.State),
		progressJSON, statsJSON, errJSON,
		t.QueuePosition, cancelFlag, []byte(t.RequestPayload),
		t.CreatedAt, t.StartedAt, t.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

func (s *postgresTaskStore) Get(ctx context.Context, taskID string) (*model.Task, error) {
	sql := `SELECT ` + taskColumns + ` FROM tasks WHERE task_id = $1`
	row := s.pool.QueryRow(ctx, sql, taskID)
	t, err := s.scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrTaskNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *postgresTaskStore) UpdateState(ctx context.Context, taskID string, patch model.TaskPatch) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update state: %w", err)
	}
	defer tx.Rollback(ctx)

	var curState string
	if err := tx.QueryRow(ctx, `SELECT state FROM tasks WHERE task_id = $1 FOR UPDATE`, taskID).Scan(&curState); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ErrTaskNotFound
		}
		return fmt.Errorf("select task state: %w", err)
	}

	if patch.State != nil {
		// 幂等：同状态重复写入视为 no-op（at-least-once 消费下，CAS 抢占后 Pipeline 首个
		// report 会重复写首阶段状态；硬约束 #18 幂等消费语义）。
		if *patch.State != model.TaskState(curState) {
			if !model.CanTransition(model.TaskState(curState), *patch.State) {
				return model.ErrInvalidTaskState
			}
		}
		if patch.State.IsTerminal() && patch.FinishedAt == nil {
			now := time.Now().UTC()
			patch.FinishedAt = &now
		}
		if *patch.State == model.TaskStateFailed && patch.Err == nil {
			return fmt.Errorf("task: failed state requires Err")
		}
	}

	sets := []string{"updated_at = now()"}
	args := []any{}
	next := 1

	add := func(column string, value any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", column, next))
		args = append(args, value)
		next++
	}

	if patch.State != nil {
		add("state", string(*patch.State))
	}
	if patch.Progress != nil {
		b, _ := json.Marshal(patch.Progress)
		add("progress_json", b)
	}
	if patch.Stats != nil {
		b, _ := json.Marshal(patch.Stats)
		add("stats_json", b)
	}
	if patch.ClearErr {
		sets = append(sets, "error_json = NULL")
	} else if patch.Err != nil {
		b, _ := json.Marshal(patch.Err)
		add("error_json", b)
	}
	if patch.QueuePosition != nil {
		add("queue_position", *patch.QueuePosition)
	}
	if patch.StartedAt != nil {
		add("started_at", *patch.StartedAt)
	}
	if patch.FinishedAt != nil {
		add("finished_at", *patch.FinishedAt)
	}

	args = append(args, taskID)
	sql := fmt.Sprintf("UPDATE tasks SET %s WHERE task_id = $%d", strings.Join(sets, ", "), next)
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("update task state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.ErrTaskNotFound
	}
	return tx.Commit(ctx)
}

func (s *postgresTaskStore) List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error) {
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	where, args := s.buildWhere(f)
	offset := int64((page - 1) * pageSize)

	var total int64
	countSQL := "SELECT COUNT(*) FROM tasks" + where
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	listArgs := append(args, pageSize, offset)
	listSQL := fmt.Sprintf("SELECT %s FROM tasks%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", taskColumns, where, len(args)+1, len(args)+2)
	rows, err := s.pool.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		t, err := s.scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, total, nil
}

func (s *postgresTaskStore) buildWhere(f model.TaskFilter) (string, []any) {
	conds := []string{}
	args := []any{}
	next := 1
	if f.Type != nil {
		conds = append(conds, fmt.Sprintf("type = $%d", next))
		args = append(args, string(*f.Type))
		next++
	}
	if f.State != nil {
		conds = append(conds, fmt.Sprintf("state = $%d", next))
		args = append(args, string(*f.State))
		next++
	}
	if f.RepoID != "" {
		conds = append(conds, fmt.Sprintf("repo_id = $%d", next))
		args = append(args, f.RepoID)
		next++
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *postgresTaskStore) SetCancelFlag(ctx context.Context, taskID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET cancel_flag = 1, updated_at = now() WHERE task_id = $1`, taskID)
	if err != nil {
		return fmt.Errorf("set cancel flag: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.ErrTaskNotFound
	}
	return nil
}

func (s *postgresTaskStore) FindInterrupted(ctx context.Context) ([]*model.Task, error) {
	sql := `SELECT ` + taskColumns + ` FROM tasks
WHERE state NOT IN ('completed','failed','cancelled')
ORDER BY created_at ASC`
	rows, err := s.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("find interrupted: %w", err)
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		t, err := s.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interrupted: %w", err)
	}
	return tasks, nil
}

// FindStale 周期恢复用：查出 updated_at 早于 staleBefore 的非终态任务（queue.RecoveryStore 契约）。
func (s *postgresTaskStore) FindStale(ctx context.Context, staleBefore time.Time) ([]*model.Task, error) {
	sql := `SELECT ` + taskColumns + ` FROM tasks
WHERE state NOT IN ('completed','failed','cancelled') AND updated_at < $1
ORDER BY created_at ASC`
	rows, err := s.pool.Query(ctx, sql, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("find stale: %w", err)
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		t, err := s.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale: %w", err)
	}
	return tasks, nil
}

// ResetStaleRunning 启动恢复扩展（queue.RecoveryStore 契约）：running 态且 updated_at 早于
// staleBefore（默认 5 分钟无心跳）→ 重置 pending（幂等 CAS：WHERE state NOT IN 终态，硬约束 #18）。
func (s *postgresTaskStore) ResetStaleRunning(ctx context.Context, staleBefore time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET state = 'pending', updated_at = now()
WHERE state NOT IN ('completed','failed','cancelled','pending') AND updated_at < $1`,
		staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("reset stale running: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ClaimPending 原子抢占：state='pending' → toState（同时写 started_at）。
// 返回 true 表示抢占成功；false 表示任务已非 pending（被别的 worker 抢占或已取消）。
func (s *postgresTaskStore) ClaimPending(ctx context.Context, taskID string, toState model.TaskState) (bool, error) {
	if !model.CanTransition(model.TaskStatePending, toState) {
		return false, fmt.Errorf("invalid pending->%s transition", toState)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET state = $2, started_at = now(), updated_at = now()
WHERE task_id = $1 AND state = 'pending'`,
		taskID, string(toState),
	)
	if err != nil {
		return false, fmt.Errorf("claim pending: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ResetToPending 重试路径：非终态任务重置回 pending（配合 DLX 重试链重投）。
func (s *postgresTaskStore) ResetToPending(ctx context.Context, taskID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET state = 'pending', updated_at = now()
WHERE task_id = $1 AND state NOT IN ('completed','failed','cancelled')`,
		taskID,
	)
	if err != nil {
		return fmt.Errorf("reset to pending: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.ErrTaskNotFound
	}
	return nil
}

// FailTask 把任务直接置为 failed（用于投递失败、重试耗尽、执行器缺失等基础设施路径），
// 不经过 CanTransition 检查（pending 任务也可能因队列满/投递失败而失败）。
func (s *postgresTaskStore) FailTask(ctx context.Context, taskID string, e *model.TaskError) error {
	errJSON, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal task error: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET state = 'failed', error_json = $2, finished_at = now(), updated_at = now()
WHERE task_id = $1 AND state NOT IN ('completed','failed','cancelled')`,
		taskID, errJSON,
	)
	if err != nil {
		return fmt.Errorf("fail task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.ErrTaskNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (s *postgresTaskStore) scanTask(row scanner) (*model.Task, error) {
	var t model.Task
	var repoID *string
	var progressJSON []byte
	var statsJSON []byte
	var errJSON []byte
	var reqPayload []byte
	var cancelFlag int

	if err := row.Scan(
		&t.TaskID, &t.Type, &repoID, &t.State,
		&progressJSON, &statsJSON, &errJSON,
		&t.QueuePosition, &cancelFlag, &reqPayload,
		&t.CreatedAt, &t.StartedAt, &t.FinishedAt,
	); err != nil {
		return nil, err
	}

	if repoID != nil {
		t.RepoID = *repoID
	}
	t.CancelFlag = cancelFlag != 0
	t.RequestPayload = reqPayload
	if err := json.Unmarshal(progressJSON, &t.Progress); err != nil {
		return nil, fmt.Errorf("unmarshal progress: %w", err)
	}
	if err := json.Unmarshal(statsJSON, &t.Stats); err != nil {
		return nil, fmt.Errorf("unmarshal stats: %w", err)
	}
	if errJSON != nil {
		t.Err = &model.TaskError{}
		if err := json.Unmarshal(errJSON, t.Err); err != nil {
			return nil, fmt.Errorf("unmarshal task error: %w", err)
		}
	}
	return &t, nil
}
