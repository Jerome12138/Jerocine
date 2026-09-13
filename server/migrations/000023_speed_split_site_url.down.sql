-- 000023 回滚: 还原 client_only, 移除测速拆分/站点网址新列。

ALTER TABLE source_health
  DROP COLUMN ad_filter_checked_at,
  DROP COLUMN ad_filter_ok,
  DROP COLUMN play_latency_web,
  DROP COLUMN sample_m3u8,
  DROP COLUMN play_checked_at,
  DROP COLUMN api_checked_at;

ALTER TABLE collect_source
  ADD COLUMN client_only TINYINT(1) NOT NULL DEFAULT 0;

ALTER TABLE collect_source DROP COLUMN site_url;
