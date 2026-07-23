-- 000002_tasks_updated_at.up.sql
-- tasks 表补充 updated_at 心跳列，支撑 Reconciler 判定 running 僵死任务（总纲 §4.3）。
-- 迁移文件一旦合入不得修改，只前进无回滚（变更总纲 §4.1）。

ALTER TABLE IF EXISTS tasks
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasks(updated_at);
