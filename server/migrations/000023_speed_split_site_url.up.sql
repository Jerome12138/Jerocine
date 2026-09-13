-- 000023 测速拆分 + 采集源站点网址 + 删除 client_only。
-- 背景(2026-09-12 方案讨论):
--   1) 测速拆两类分开测/显示: 采集速度(服务端打采集 API) 与 播放速度(浏览器端直连 CDN 计时)。
--      source_health 记录各自检测时间; 浏览器端测速结果回传落库。
--   2) 服务端抽样 m3u8 的 MeasureURL 结果语义改为「服务端 m3u8 可达性」→ 决定服务端代理
--      广告过滤链路是否可用(ad_filter_ok), 播放页据此跳过死链路。
--   3) client_only 整体移除: 服务端够不着的源永远采不到片 → 不产生播放线路 → 无业务价值,
--      只剩特判负担。存量 client_only 源回归普通源, 探测失败会按正常逻辑计失败/自动停采。
--   4) collect_source 增加可选站点网址(后台展示/跳转用)。

ALTER TABLE collect_source
  ADD COLUMN site_url VARCHAR(512) NOT NULL DEFAULT '' AFTER uri;

ALTER TABLE collect_source DROP COLUMN client_only;

ALTER TABLE source_health
  ADD COLUMN api_checked_at BIGINT NOT NULL DEFAULT 0 AFTER checked_at,
  ADD COLUMN play_checked_at BIGINT NOT NULL DEFAULT 0 AFTER api_checked_at,
  ADD COLUMN sample_m3u8 VARCHAR(1024) NOT NULL DEFAULT '' AFTER play_checked_at,
  ADD COLUMN play_latency_web BIGINT NOT NULL DEFAULT 0 AFTER sample_m3u8,
  ADD COLUMN ad_filter_ok TINYINT(1) NULL AFTER play_latency_web,
  ADD COLUMN ad_filter_checked_at BIGINT NOT NULL DEFAULT 0 AFTER ad_filter_ok;
