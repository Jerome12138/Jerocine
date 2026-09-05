-- 000007 采集源健康度扩展: 目录大小(资源最全判定) + 抽样播放延时(线路排序)。
ALTER TABLE source_health
  ADD COLUMN page_count     INT    NOT NULL DEFAULT 0 AFTER films,
  ADD COLUMN total          INT    NOT NULL DEFAULT 0 AFTER page_count,
  ADD COLUMN play_latency_ms BIGINT NOT NULL DEFAULT 0 AFTER total;
