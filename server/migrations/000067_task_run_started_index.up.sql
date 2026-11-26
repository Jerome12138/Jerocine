-- 000067 任务台账按 started_at 统计补索引（任务中心 /overview 的 24h 失败计数）
--
-- 背景：CountSince 的查询是 `WHERE started_at >= ?` 做 SUM(CASE ...)，而 task_run 现有索引是
--   idx_task_run_created(created_at) / idx_task_run_status(status, created_at) /
--   idx_task_run_cron(kind, cron_id) / idx_status_ignored_started(status, ignored, started_at)。
-- 最后一条最左列是 status，纯 started_at 范围条件用不上 ⇒ 30 天保留期下**全表扫**，
-- 而 /manage/task-center/overview 是页面轮询接口（线上台账量级：几十万行起）。
--
-- 保留 30 天口径不变，只补一条能覆盖范围扫描的索引。

ALTER TABLE task_run
  ADD KEY idx_started (started_at);
