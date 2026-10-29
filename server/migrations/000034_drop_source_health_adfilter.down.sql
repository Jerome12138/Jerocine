-- 恢复 000023 加列时的原始定义(数据不可恢复, 仅还原表结构)。
ALTER TABLE source_health
  ADD COLUMN ad_filter_ok TINYINT(1) NULL AFTER play_latency_web,
  ADD COLUMN ad_filter_checked_at BIGINT NOT NULL DEFAULT 0 AFTER ad_filter_ok;
