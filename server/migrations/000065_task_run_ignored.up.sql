-- 000065 任务台账支持"忽略"（统一任务中心的异常处理）
--
-- 背景：所有定时任务的异常都要能在"异常"页看到，并支持**重新执行**或**忽略**。
-- 忽略 = 打标记（不删行），保留审计；列表默认不展示已忽略项，可按需查看。

ALTER TABLE task_run
  ADD COLUMN ignored TINYINT(1) NOT NULL DEFAULT 0 COMMENT '已忽略（异常处理：不再提示）' AFTER error,
  ADD KEY idx_status_ignored_started (status, ignored, started_at);
