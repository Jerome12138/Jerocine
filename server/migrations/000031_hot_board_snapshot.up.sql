-- 000031 热榜快照与轮播候选表 (见 docs/热榜页面与轮播候选方案-2026-10-23)。
--
-- 背景:
--   ① 独立热榜页 /hotBoard 需要按"某个榜的原始位次"浏览 —— 而 movie.hot_rank 只存每部片的
--      最优榜位(同片多榜被去重), 无法还原"某榜的完整名次", 故需要一份逐榜逐位的快照;
--   ② 首页轮播候选要"8 榜各取前 2、加权轮转混排、大陆保底", 每日刷新算好后直接落库,
--      compose 一次 SELECT 取走, 不做实时重算。
--
-- 两表都是每日榜单刷新(HotService.Refresh)成功后**全量替换**(DELETE ALL + 批量 INSERT):
--   本轮抓取失败/不达标 → 不写, 保留旧快照, 轮播与热榜页继续用昨天的;
--   它们是派生数据, 随时可从豆瓣榜单 + movie_search 重建, 不做历史版本。

CREATE TABLE IF NOT EXISTS hot_board_item (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  board      VARCHAR(64)  NOT NULL DEFAULT '',  -- 豆瓣集合名(如 movie_hot_gaia)
  `rank`     INT          NOT NULL DEFAULT 0,   -- 该榜位次(从 1 起)
  mid        BIGINT       NOT NULL DEFAULT 0,   -- 关联 movie_search.mid
  refresh_at BIGINT       NOT NULL DEFAULT 0,   -- 本轮刷新时间(Unix 秒)
  UNIQUE KEY uq_board_rank (board, `rank`),     -- 同榜每一位只允许一行
  KEY idx_mid (mid)                             -- 按片反查命中的榜(多榜回血用)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS hot_board_candidate (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  weight_pos INT          NOT NULL DEFAULT 0,   -- 候选池排序位(0..15), 轮播按此顺序取
  board      VARCHAR(64)  NOT NULL DEFAULT '',  -- 来源榜
  `rank`     INT          NOT NULL DEFAULT 0,   -- 来源榜位次
  mid        BIGINT       NOT NULL DEFAULT 0,
  score      INT          NOT NULL DEFAULT 0,   -- 候选分(含 TC 降级/多榜回血/地区年份)
  guaranteed TINYINT(1)   NOT NULL DEFAULT 0,   -- 1=中国大陆保底替换位
  refresh_at BIGINT       NOT NULL DEFAULT 0,
  UNIQUE KEY uq_weight_pos (weight_pos),
  KEY idx_mid (mid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
