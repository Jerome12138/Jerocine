-- 000006 采集源健康度: 一源一行最新快照 + 连续失败计数 + 自动停采标记。
-- 由健康检查(手动测速/定时任务)写入; suppressed=1 表示连续失败达阈值被自动停采(与 collect_source.state 正交)。
CREATE TABLE IF NOT EXISTS source_health (
  source_id         VARCHAR(32)  NOT NULL PRIMARY KEY,
  status            VARCHAR(16)  NOT NULL DEFAULT 'unknown',  -- healthy / degraded / down / unknown
  consecutive_fails INT          NOT NULL DEFAULT 0,
  suppressed        TINYINT      NOT NULL DEFAULT 0,          -- 1 = 自动停采(健康标记, 不动 state)
  last_ok           TINYINT      NOT NULL DEFAULT 0,
  latency_ms        BIGINT       NOT NULL DEFAULT 0,          -- 最近一次成功探测的中位延时
  best_ms           BIGINT       NOT NULL DEFAULT 0,
  films             INT          NOT NULL DEFAULT 0,
  ok_count          INT          NOT NULL DEFAULT 0,
  probes            INT          NOT NULL DEFAULT 0,
  message           VARCHAR(255) NOT NULL DEFAULT '',
  checked_at        BIGINT       NOT NULL DEFAULT 0,
  updated_at        BIGINT       NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
