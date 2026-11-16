-- 000046：给派生表加**覆盖索引** (match_key, epid, site_id)。
--
-- 背景（2026-11-11 线上 524 事故的优化，真库实测）：
--   `GROUP BY match_key, epid` + `COUNT(DISTINCT site_id)` 是"对齐率"系列指标的核心形态；
--   现有索引 `idx_match_epid(match_key, epid)` **不含 site_id** ⇒ 优化器直接放弃索引，
--   退化为 `type=ALL` 全表扫 + `Using temporary`（196 万行实测 **19.7s**）✗
--   而只用索引内两列时是 `Using index`（纯覆盖，**6.5s**）✓
-- ⇒ 把 site_id 一并放进索引即可让分组在**索引内**完成（不回表、不需临时表），①②③④ 预计 25s → 3~8s ✓
--
-- 代价：索引体积增加（≈派生表的 1/4）与写入略慢；派生表是派生物、写入低频（每批 500 源行）⇒ 可接受 ✓
-- 建索引在 196 万行上预计 10~40s（单次维护操作）。
ALTER TABLE movie_episode_epid
    ADD INDEX idx_match_epid_site (match_key, epid, site_id);
