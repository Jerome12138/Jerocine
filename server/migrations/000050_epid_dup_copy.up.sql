-- 000050：双写副本标记 `dup_copy`（统计口径排除用，2026-11-13）。
--
-- 背景：补充源同片按 `hash(片名)/hash(豆瓣ID)` 双键各存一行（`server/internal/spider/parse.go`
--   `buildPlaySources`）⇒ 同一上游视频 = **两条独立源行** ⇒ 跨键重复 / 同一集两份 epid 不同步(2.19%)
--   / 对齐率读数被抬高（实测含副本 35.16% vs 排除副本 32.83%，虚高 ≈2.3pp）。
--   决策：**暂保持双行**，改从**统计口径排除副本**。
--
-- 标记依据（**写侧零判断**）：`buildPlaySources` 生成 `keys` 时**就知道每个键的来源** ——
--   `keys[0] = hash(d.VodName)`（片名键 ⇒ 主份 0）、`keys[1] = hash(d.VodDouBanID)`（豆瓣键 ⇒ 副本 1）。
--   故写侧**顺手落库**，不需要任何"算"或"判"。
--
-- 两处存放（受控冗余，与 mid/match_key/site_id/play_from 同理）：
--   · `movie_play_source.dup_copy`  = **真相**（源行的属性，写侧落 + 存量脚本补）
--   · `movie_episode_epid.dup_copy` = **冗余**（由派生**继承**；指标按它过滤，免 JOIN 巨表）
--
-- ⚠️ 为什么指标不能只查源表：对齐率是**全表 index-only 聚合**（6.5M 行，靠覆盖索引在索引内分组）；
--   一旦要 JOIN `movie_play_source` 才能过滤 ⇒ 退化为临时表 ⇒ 历史上正是这条路把接口拖到 **524**。
--
-- 存量补齐（**不需要全站重采**）：离线脚本按 `键 ∈ {hash(movie.db_id)}` 反推（命中的键＝豆瓣键 ⇒ 标 1，
--   另一键标 0）；**判不了的组整组不动**（保持 0 = 不排除，宁缺勿错）。实测可判 ≈67%；
--   将来 `source_douban_id` 落库后可判到 ~100%（判据即成为写侧那一条）。收敛：重采/重算会逐步用**正确值**覆盖。
--
-- 索引：把 `dup_copy` 放在**新索引最前**，`WHERE dup_copy = 0 GROUP BY match_key, epid` 仍是 `Using index` ✓
ALTER TABLE movie_play_source
    ADD COLUMN dup_copy TINYINT NOT NULL DEFAULT 0 COMMENT '双写副本标记：0=片名键(VodName)主份 / 1=豆瓣键(VodDouBanID)副本（口径排除用，000050）';

ALTER TABLE movie_episode_epid
    ADD COLUMN dup_copy TINYINT NOT NULL DEFAULT 0 COMMENT '继承自源行的 dup_copy（0=主份 / 1=副本）';

ALTER TABLE movie_episode_epid
    ADD INDEX idx_dup_match_epid_site (dup_copy, match_key, epid, site_id);
