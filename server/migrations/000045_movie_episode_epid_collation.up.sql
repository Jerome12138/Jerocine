-- 000045：把派生表 movie_episode_epid 的字符集/排序规则**对齐源表** movie_play_source。
--
-- 背景（2026-11-11 审查 N1 的根因）：建表迁移 000044 未显式指定 collation ⇒ 落在服务器默认
-- `utf8mb4_0900_ai_ci`，而 movie_play_source 的列是 `utf8mb4_unicode_ci` ⇒ **任何"派生表列 vs 源表列"
-- 的比较/join 都会报 Illegal mix of collations** ✗（只有"列 vs 字面量/参数"的比较没事，所以既有的
-- ListByMatchKeys 一直正常，直到指标层写了跨表比较才炸）。
--
-- 本迁移把整表转成 utf8mb4_unicode_ci（与源表一致）⇒ 以后不必在每个查询里手写 COLLATE。
-- 代价：CONVERT 会重建表 + 索引 —— 派生表是**可重建的派生物**且当前仅约 200 万行，秒级~分钟级 ✓
-- 注意：此后新写入的 epid/raw_name 等也随表 collation ⇒ 与旧行一致 ✓
ALTER TABLE movie_episode_epid
    CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
