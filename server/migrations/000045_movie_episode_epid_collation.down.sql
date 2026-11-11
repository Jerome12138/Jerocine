-- 000045 down：把派生表转回服务器默认（utf8mb4_0900_ai_ci）。
--
-- 说明：本迁移只影响**跨表列比较**（列 vs 参数不受影响）。回滚到 0900_ai_ci 会让
-- "派生表列 vs 源表列"的比较重新报 Illegal mix of collations ⇒ 指标层的 AlignmentRateComparable
-- 里已带显式 COLLATE，故回滚后仍可运行 ✓（其它未写 COLLATE 的新查询会踩 ✗）。
ALTER TABLE movie_episode_epid
    CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
