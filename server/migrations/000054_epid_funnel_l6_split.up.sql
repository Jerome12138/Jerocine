-- 000054 L6 拆分：纯 n / 纯 d 整片 / 纯 d 按槽（S1，2026-11-18）
--
-- 背景：L6 原为单一"纯 n / 纯 d 容忍"，整片级判定——任意一行带 s/v 就整片放弃，
--       导致 0.1% 的脏行把整片干净槽一起打成未对齐（综艺实测 31,408 槽）。
-- 变更：拆成三个子档并分别落库，L6Tol 仍为三者之和（兼容旧消费方）。
--   l6_num  = L6.1 纯 n 整片
--   l6_date = L6.2 纯 d 整片（原行为）
--   l6_slot = L6.3 纯 d 按槽（新增：槽内每行纯 d 且该日期在本片任一源只出现 1 次）
ALTER TABLE epid_alignment_funnel_snapshot
  ADD COLUMN l6_num  BIGINT NOT NULL DEFAULT 0 COMMENT 'L6.1 纯n整片容忍槽' AFTER l6_tol,
  ADD COLUMN l6_date BIGINT NOT NULL DEFAULT 0 COMMENT 'L6.2 纯d整片容忍槽' AFTER l6_num,
  ADD COLUMN l6_slot BIGINT NOT NULL DEFAULT 0 COMMENT 'L6.3 纯d按槽容忍槽（同日唯一守卫）' AFTER l6_date;