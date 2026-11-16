-- 000051 down：移除 `content_hash`（口径退回"每次都写整行"，binlog 体积回升）。
--
-- 回滚注意：`play_repo.UpsertBatch` 的判重与 `DoUpdates` 里都引用了该列 ⇒ **必须连同代码一起回滚**
-- （实体 `MoviePlaySource.ContentHash`、`EpisodesContentHash`、UpsertBatch 的判重与更新列）。
ALTER TABLE movie_play_source DROP COLUMN content_hash;
