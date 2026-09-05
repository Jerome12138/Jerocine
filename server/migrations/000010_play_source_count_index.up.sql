-- 000010 movie_play_source 加覆盖索引 (site_id, source_vod_id, match_key)。
-- CountBySite 去重统计仅需这三列, 但无此索引时 GROUP BY 会全表扫描 → 连带读取巨大的
-- episodes_json(JSON 大列), 实测 30~60s 甚至超时。覆盖索引使查询 index-only(不回表),
-- 数百毫秒完成。InnoDB online DDL, 加索引期间不阻塞采集写入。
CREATE INDEX idx_mps_site_vod_match ON movie_play_source (site_id, source_vod_id, match_key);
