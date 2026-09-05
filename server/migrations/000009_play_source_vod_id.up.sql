-- 000009 movie_play_source 增加源站影片ID(source_vod_id)。
-- 背景: 补充源(入库 mid==nil)同一部片会按 hash(片名) 与 hash(豆瓣ID) 双键各存一行,
--   仅凭 match_key 无法判定"同一部片", 故 COUNT(*) 行数 > 去重片数(sn 实测 1.43x)。
-- 落源站 vod_id 后, (site_id, source_vod_id) 唯一定位一部片 → 可按它 COUNT(DISTINCT) 去重统计。
-- 旧行默认 0(未知); 下次采集 upsert 回填(UpsertBatch 已把 source_vod_id 纳入更新列);
--   统计时 vod_id=0 的旧行回退按 match_key 计, 全量再采一轮后自然收敛。
ALTER TABLE movie_play_source
  ADD COLUMN source_vod_id BIGINT NOT NULL DEFAULT 0 AFTER match_key;
