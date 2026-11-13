-- 000050：双写副本标记 `dup_copy`（统计口径排除用，2026-11-13）。
--
-- 背景：补充源同片按 `hash(片名)/hash(豆瓣ID)` **双键各存一行**（`server/internal/spider/parse.go`
--   `buildPlaySources`）⇒ 同一个上游视频 = **两条独立源行** ⇒ 派生/判脏/借用池各自一份，带来
--   「跨键重复」「同一集两份 epid 不同步（实测 2.19%）」「对齐率读数被抬高」。
--   实测（同口径子集）：**含副本 35.16% vs 排除副本 32.83%** —— 见
--   `docs/epid-集身份/分析-采集侧键口径与匹配度-2026-11-13.md` §10.5 / §11。
--
-- 决策（同上 §10.5）：**暂保持双行**（不改采集/表结构语义），改从**统计口径排除副本**。
--
-- 标记**就地可判**：`keys[0]` 恒为片名键（主份 = 0），其余键（豆瓣键）= 副本 1。故写侧即知，
-- 无需 JOIN 巨表、也无需算 `MIN(id)`。
--
-- 索引：对齐率系列是 **index-only** 计划（见 000046 的 `idx_match_epid_site`）；
--   把 `dup_copy` 放在**新索引最前**，`WHERE dup_copy = 0 GROUP BY match_key, epid` 仍是 `Using index` ✓
--   （不加这条索引 ⇒ 过滤列不在覆盖索引里 ⇒ 回表 ⇒ 该指标重新退化，可能再把接口拖到 524）。
--
-- ⚠️ 存量数据：本迁移**只回填源表**（约 8.6 万行副本，秒级）。
--   `movie_episode_epid`（约 650 万行）**不在迁移里回填**（避免大表 UPDATE 拖死迁移）；它由
--   **重算自然收敛**（派生写侧已带 `dup_copy`），或按 `src_row_id` 分批回填，SQL 见本文件末尾注释。
ALTER TABLE movie_play_source
    ADD COLUMN dup_copy TINYINT NOT NULL DEFAULT 0 COMMENT '双写副本标记：0=片名键主份 / 1=豆瓣键副本（口径排除用，000050）';

ALTER TABLE movie_episode_epid
    ADD COLUMN dup_copy TINYINT NOT NULL DEFAULT 0 COMMENT '同源行 dup_copy（0=主份 / 1=副本）';

ALTER TABLE movie_episode_epid
    ADD INDEX idx_dup_match_epid_site (dup_copy, match_key, epid, site_id);

-- 存量回填（源表）：同一 `(site_id, source_vod_id, play_from)` 只保留 `MIN(id)` 为主份，其余=副本。
-- `source_vod_id = 0` 的旧行无副本概念，保持 0。
UPDATE movie_play_source s
JOIN (
    SELECT MIN(id) AS keep_id, site_id, source_vod_id, play_from
    FROM movie_play_source
    WHERE source_vod_id <> 0
    GROUP BY site_id, source_vod_id, play_from
) t
  ON t.site_id = s.site_id
 AND t.source_vod_id = s.source_vod_id
 AND t.play_from = s.play_from
SET s.dup_copy = 1
WHERE s.id <> t.keep_id;

-- 派生表存量回填（**分批执行**，勿整表一条 SQL；每批按 src_row_id 区间推进，可续跑）：
--   UPDATE movie_episode_epid e
--     JOIN movie_play_source s ON s.id = e.src_row_id
--     SET e.dup_copy = s.dup_copy
--    WHERE e.src_row_id BETWEEN ? AND ?;
