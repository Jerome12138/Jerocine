-- 榜单热度: 记录 hot_rank 的来源榜单(豆瓣集合名, 如 movie_hot_gaia)。
-- 背景: 同一部片会在多个集合出现(热门电影 No.2 / 正在上映 No.2 / 一周口碑榜 No.5),
-- 现行取"最好位次"落库, 但页面只能看到 No.2, 看不出是哪个榜的 No.2。
-- hot_board 与 hot_rank 同生命周期: 刷新时随在榜行写入, 掉榜行清空。
ALTER TABLE movie
  ADD COLUMN hot_board VARCHAR(32) NOT NULL DEFAULT '' AFTER hot_rank_at;

ALTER TABLE movie_search
  ADD COLUMN hot_board VARCHAR(32) NOT NULL DEFAULT '' AFTER hot_rank_at;
