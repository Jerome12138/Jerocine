-- 000020: 榜单热度列 —— 由豆瓣榜单刷新任务写入(见 service/hot_service.go)。
--
--   hot_rank     当前榜位, 0 = 不在榜
--   hot_rank_at  本轮抓取时间(Unix 秒, 与 movie.update_stamp 同量纲), 0 = 从未刷新
--   hot_score    合成热度分 = 榜位分(0 或 60-100) + 更新活跃分(0-20) + 口碑分(0-10)
--   db_id_src    movie.db_id 的来源标记: 0 = 源站自带 / 1 = 榜单回填
--
-- 四列都是"本地计算 / 本地回填列", 源站没有任何对应数据 —— 故全部进 upsert 排除集
-- (movieUpsertExclude / searchUpsertExclude), 源站重推不会把算好的热度抹回 0。
-- movie_search 是"全量重采时的影子表重建"读模型, 换表前需照 backdrop 模式从 movie 回灌,
-- 否则一次全量就会把热度集体清空(见 search_repo.go 的 ShadowCommit)。
--
-- idx_hot 的列顺序与查询排序键一致(hot_score DESC, year DESC, update_stamp DESC, mid DESC),
-- 让"热度优先/最新上线"排序全程走索引。教训: movie_search 上无索引的 update_stamp
-- 排序在生产日志里实测耗时 407ms。

ALTER TABLE movie
  ADD COLUMN hot_rank    INT     NOT NULL DEFAULT 0 AFTER backdrop,
  ADD COLUMN hot_rank_at BIGINT  NOT NULL DEFAULT 0 AFTER hot_rank,
  ADD COLUMN hot_score   INT     NOT NULL DEFAULT 0 AFTER hot_rank_at,
  ADD COLUMN db_id_src   TINYINT NOT NULL DEFAULT 0 AFTER hot_score,
  ADD INDEX idx_hot (hot_score, year, update_stamp, mid);

ALTER TABLE movie_search
  ADD COLUMN hot_rank    INT     NOT NULL DEFAULT 0 AFTER backdrop,
  ADD COLUMN hot_rank_at BIGINT  NOT NULL DEFAULT 0 AFTER hot_rank,
  ADD COLUMN hot_score   INT     NOT NULL DEFAULT 0 AFTER hot_rank_at,
  ADD COLUMN db_id_src   TINYINT NOT NULL DEFAULT 0 AFTER hot_score,
  ADD INDEX idx_hot (hot_score, year, update_stamp, mid);
