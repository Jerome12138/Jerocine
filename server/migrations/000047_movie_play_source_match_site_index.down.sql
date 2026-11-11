-- 000047 down：删掉 (match_key, site_id) 覆盖索引。
-- 回滚后"多源片子查询"会退回 回表 + 15s（可比口径对齐率整条会变慢），
-- 但接口侧的"每项独立超时 + 全项容错"仍在（88f2c7e）⇒ 只会降级该项、不会 500 ✓
ALTER TABLE movie_play_source
    DROP INDEX idx_match_site;
