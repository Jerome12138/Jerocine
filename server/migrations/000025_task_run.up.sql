-- 000025_task_run.up.sql — 任务运行台账(持久化任务历史/失败原因/重跑依据)
-- 每次任务触发一行: cron 触发(kind=cron)与手动触发(kind=manual)统一登记。
CREATE TABLE IF NOT EXISTS task_run (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  kind       VARCHAR(16)  NOT NULL COMMENT '触发来源: cron | manual',
  task_type  VARCHAR(24)  NOT NULL COMMENT 'collect | recover | category_cover | hot_refresh',
  cron_id    BIGINT       NOT NULL DEFAULT 0 COMMENT '定时任务 id(非定时触发为 0)',
  source_id  VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '采集源 id(采集/覆盖类)',
  name       VARCHAR(128) NOT NULL DEFAULT '' COMMENT '任务显示名',
  hours      INT          NOT NULL DEFAULT -1 COMMENT '采集时长: -1 全量 / >0 增量小时',
  status     VARCHAR(16)  NOT NULL DEFAULT 'running' COMMENT 'running | success | failed | canceled',
  total      INT          NOT NULL DEFAULT 0 COMMENT '总页数',
  done       INT          NOT NULL DEFAULT 0 COMMENT '成功页数',
  failed     INT          NOT NULL DEFAULT 0 COMMENT '失败页数',
  message    VARCHAR(255) NOT NULL DEFAULT '' COMMENT '结果摘要(如 11 源成功 2 失败, 采 1200 页)',
  error      VARCHAR(512) NOT NULL DEFAULT '' COMMENT '失败原因(截断)',
  started_at BIGINT       NOT NULL DEFAULT 0 COMMENT '开始时间(ms)',
  ended_at   BIGINT       NOT NULL DEFAULT 0 COMMENT '结束时间(ms)',
  created_at BIGINT       NOT NULL DEFAULT 0,
  updated_at BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_task_run_created (created_at),
  KEY idx_task_run_status (status, created_at),
  KEY idx_task_run_cron (kind, cron_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务运行台账';
