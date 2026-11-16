-- 000052 down：移除「上游元数据」两列（双写副本判定退回"只能反推 ≈69.8%"）。
--
-- 回滚前已知会丢什么：`source_name` / `source_douban_id` 是**上游原文快照**，
-- 只用于离线判定双写副本 + 排查；删掉后这些行只能等下次重采再采回来（存量本来就多为 NULL）。
-- 比对逻辑（`play_repo.playSourceUpsertUpdates`）若不同步回滚，会引用不存在的列 ⇒ **必须连同代码一起回滚**。
ALTER TABLE movie_play_source
    DROP COLUMN source_douban_id,
    DROP COLUMN source_name;
