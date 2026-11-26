-- 000066 采集定时任务改用统一的时间语义（友好文本），由任务中心统一调度
--
-- 背景：内置任务用友好文本（daily:04:00），采集任务用 cron 表达式（0 0 4 * * 0），
-- 两套语义并存导致「配置入口两个」「中心算不了下次时间」。本迁移给 cron_task 加 schedule 列，
-- 存量 spec 由服务启动时**幂等转换**（见 internal/taskcenter/cronconv.go）：
--   0 */20 * * * ?  → every:20m
--   0 0 4 * * 0     → weekly:7 04:00
-- 无法识别的 spec 会被停用并标注，等人工在任务中心改周期（不静默跑错）。

ALTER TABLE cron_task
  ADD COLUMN schedule VARCHAR(64) NOT NULL DEFAULT '' COMMENT '友好文本周期（统一时间语义）；空=待转换/未配置' AFTER spec;
