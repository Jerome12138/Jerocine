-- 000013 采集页级失败台账。
-- 背景: 采集引擎对页级失败此前只打日志 + 累加一个计数, 失败页里的影片就此永久丢失,
--   而且事后无从知道"哪一页、什么参数"丢了(健康度管的是源级, 覆盖不到页级)。
-- 落成台账后失败页可被补采: 增量失败按扩大的时间窗整段重扫, 全量/超长范围按页码精确重放。
CREATE TABLE IF NOT EXISTS collect_failure (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_id  VARCHAR(32)  NOT NULL,
  page_no    INT          NOT NULL DEFAULT 0,
  hours      INT          NOT NULL DEFAULT 0,    -- 发起这次采集时的时长参数(0=全量, >0 为增量小时)
  cause      VARCHAR(512) NOT NULL DEFAULT '',
  status     TINYINT      NOT NULL DEFAULT 0,    -- 0 待补采 / 1 已处理
  attempts   INT          NOT NULL DEFAULT 0,    -- 被记录到的次数(同源同页同参数重复失败则累加)
  created_at BIGINT       NOT NULL,
  updated_at BIGINT       NOT NULL,
  KEY idx_status_created(status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 预置"每周凌晨自动补采失败页"的定时任务(model=2)。后台"定时任务"页可改 spec 或停用。
INSERT INTO cron_task (source_ids, spec, time, model, state, remark, entry_id, last_run_at, created_at, updated_at)
SELECT NULL, '0 0 4 * * 0', 0, 2, 0, '每周日凌晨 4 点自动补采失败页', 0, 0,
       UNIX_TIMESTAMP() * 1000, UNIX_TIMESTAMP() * 1000
WHERE NOT EXISTS (SELECT 1 FROM cron_task WHERE model = 2);
