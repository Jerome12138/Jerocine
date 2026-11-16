-- 000049 down：删掉裁定台账表。
--
-- ⚠️ 台账内容随表一起删除——裁定的"为什么"（reason/evidence/三元组键）不可由规则重算，
-- 丢失后只能重新人工核对。回滚前如有已录入裁定，请先导出留存。
-- 派生表 movie_episode_epid 本身不受影响（本迁移从未对它加列/改行）✓
DROP TABLE IF EXISTS movie_epid_override;
