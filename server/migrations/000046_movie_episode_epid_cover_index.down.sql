-- 000046 down：删掉覆盖索引（保留原 idx_match_epid 等）。
-- 注意：删掉后"对齐率"系列指标会退回全表扫 + 临时表（线上实测 19.7s / 曾把接口拖到 524），
-- 因此回滚需同时确认指标接口的超时/容错仍在（88f2c7e 已加）✓
ALTER TABLE movie_episode_epid
    DROP INDEX idx_match_epid_site;
