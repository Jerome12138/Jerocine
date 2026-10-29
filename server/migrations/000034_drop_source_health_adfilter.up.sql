-- 000034 清理 source_health 孤儿列 —— 服务端 m3u8 代理/过滤链路 2026-10-28 下线。
--
-- 背景: entity.SourceHealth 已删 AdFilterOk/AdFilterCheckedAt 两字段(Go 侧不再读写);
--   项目表结构走 golang-migrate(无 AutoMigrate), 旧列留在库里只是死列, 本迁移清掉。
--   保留 play_latency_ms / sample_m3u8(测播放与线路排序仍在用)。

ALTER TABLE source_health
  DROP COLUMN ad_filter_ok,
  DROP COLUMN ad_filter_checked_at;
