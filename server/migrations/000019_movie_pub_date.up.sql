-- 000019: 影片上映日期(pub_date) —— 源站 vod_pubdate 规范化后落库。
--
-- 背景: 源站 vod_pubdate 的精度并不统一, 实测形态为 "YYYY-MM-DD(地区)" / "YYYY-MM(地区)" /
-- "YYYY" 三种, 且大量为空(整体约 38%, 影视类 60-100%、AI漫剧/短剧类接近 0)。
-- 故用 VARCHAR(10) 存"ISO 前缀串"(2026-09-11 / 2026-07 / 2007 / ''):
--   * 不臆造源站未提供的精度;
--   * ISO 前缀的字典序 == 时间序, 可直接参与 ORDER BY, 不必拆成 year/month/day 三列再拼接。
-- 原 movie.year 仍由 parseYear 独立解析(兼容 vod_year 兜底), 二者不冲突。
--
-- 本列是"采集内容列", 参与采集 upsert 更新(与 hot_* 那组本地计算列相反);
-- 但会加上"源站为空则不覆盖旧值"的条件保护(见 movie_repo.go 的 B 类条件更新),
-- 避免源站这次没给日期就把上一次采到的日期抹掉。
--
-- 不回填存量: 存量影片维持空串, 随日常采集自然积累(当前不存在定期全量任务)。

ALTER TABLE movie
  ADD COLUMN pub_date VARCHAR(10) NOT NULL DEFAULT '' AFTER year;

ALTER TABLE movie_search
  ADD COLUMN pub_date VARCHAR(10) NOT NULL DEFAULT '' AFTER year;
