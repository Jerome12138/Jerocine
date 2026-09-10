-- 回滚 000013: 移除失败台账与预置的补采任务。
DELETE FROM cron_task WHERE model = 2;
DROP TABLE IF EXISTS collect_failure;
