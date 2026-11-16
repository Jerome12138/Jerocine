-- 000050 down：移除双写副本标记（口径退回"含副本"，读数会**虚高 ≈2.3pp**）。
--
-- 回滚前已知会丢什么：`dup_copy` 只是**统计口径标记**，不影响 epid 值与派生结果；
-- 但回滚后 `AlignmentRate` / `AlignmentRateComparable` 的 `WHERE dup_copy = 0` 会**报错**
-- （列不存在）⇒ **必须连同代码一起回滚**（`epid_repo.go` 的两条 SQL + 两处实体字段 +
-- `parse.go` 的写侧标记 + `play_repo.go` 的 DoUpdates/判重列）。
ALTER TABLE movie_episode_epid DROP INDEX idx_dup_match_epid_site;

ALTER TABLE movie_episode_epid DROP COLUMN dup_copy;

ALTER TABLE movie_play_source DROP COLUMN dup_copy;
