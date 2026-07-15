package task

import (
	"context"

	"deepwiki/internal/model"
)

// Executor 任务执行器：按 TaskType 注册到 TaskManager，Worker 取出任务后路由执行。
// 实现方（下一轮）：ingestExecutor（五阶段 Pipeline）/ refreshExecutor / wikiExecutor，
// 每阶段入口与循环内必须 select ctx.Done()（硬约束 #4）。
//
// 消费侧执行路径（总纲 §4.3，硬约束 #18 幂等消费）：
//  1. consumer 收到瘦消息 → 读 Postgres 校验任务仍为 pending
//     （CAS：UPDATE tasks SET state='cloning' ... WHERE task_id=$1 AND state='pending'；
//     CAS 失败 = 别的节点已抢占或任务已取消 → 直接 ack 丢弃，禁止重复执行）；
//  2. 路由 Executor.Execute 执行 pipeline，逐阶段 UpdateState + EventBus.Publish；
//  3. 终态落库成功 → ack；panic/recover 或瞬时错误 → nack requeue=false 进 DLX 重试链
//     （deepwiki.task.retry.{5s,30s,5m}，最多 queue.rabbitmq.max_retries=3 次）；
//  4. 重试耗尽 → 任务落库 failed（error.code=50003）。
type Executor interface {
	Type() model.TaskType
	Execute(ctx context.Context, t *model.Task) error
}
