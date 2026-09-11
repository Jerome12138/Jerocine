-- 000020 down: 移除榜单热度列与索引。

ALTER TABLE movie
  DROP INDEX idx_hot,
  DROP COLUMN hot_rank,
  DROP COLUMN hot_rank_at,
  DROP COLUMN hot_score,
  DROP COLUMN db_id_src;

ALTER TABLE movie_search
  DROP INDEX idx_hot,
  DROP COLUMN hot_rank,
  DROP COLUMN hot_rank_at,
  DROP COLUMN hot_score,
  DROP COLUMN db_id_src;
