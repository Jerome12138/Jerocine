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
-- ⚠️ **存量不回填（2026-11-13 评审结论，方案 A）** —— 本迁移**只加列 + 加索引，不改任何数据**。
--
-- 为什么放弃回填：回填只能按结构判据（"同一 `(site_id, source_vod_id, play_from)` 取 `MIN(id)` 为主份"），
--   而**写侧语义是"`keys[0]` = 片名键 = 主份"**（见 `spider.parse.go`）—— 两者**可能相反**（`UpsertBatch`
--   会按 `(site_id, match_key, play_from)` 排序后再插入 ⇒ `id` 序**不携带**"哪一个是片名键"的信息）。
--   实测（2 键身份 175,909）：能判定的样本里约 **3.3% 明确相反**，另有 **43% 无法判定** ⇒ 失配率未知但非零。
--   一旦回填标反，"排除副本"留下的那一半就**不是片名键** ⇒ 指标分组与未来"只看主份"的消费者都不可信。
--
-- 取舍方向：**宁可不排除，也不错排除** ⇒ 存量行一律保持 `dup_copy = 0`（视为"未标记"，即**不排除**），
--   等**重采 / 重算**自然收敛（那时写侧会按统一语义写入）。因此 `dup_copy = 0` 的含义是
--   "**未知 / 非副本**"，而非"已确认是主份" —— 消费方按此理解。
ALTER TABLE movie_play_source
    ADD COLUMN dup_copy TINYINT NOT NULL DEFAULT 0 COMMENT '双写副本标记：0=片名键主份（或存量未标记）/ 1=豆瓣键副本（口径排除用，000050）';

ALTER TABLE movie_episode_epid
    ADD COLUMN dup_copy TINYINT NOT NULL DEFAULT 0 COMMENT '同源行 dup_copy（0=主份或存量未标记 / 1=副本）';

ALTER TABLE movie_episode_epid
    ADD INDEX idx_dup_match_epid_site (dup_copy, match_key, epid, site_id);

