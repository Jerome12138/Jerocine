-- 000055 d 覆盖率入快照（M8，2026-11-19）
--
-- 用途：在漏斗里同时给出"纯日期槽总量"与"其中日期被 ≥2 源覆盖的槽数"，
--       支撑 O3 项"综艺对齐率旁标注跨源一致率"，并为 M5 抽样复用。
-- 口径：d_slots_total = 槽内每一行都是纯日期（d 非空且 n=0）；
--       d_slots_cross = 上述槽中"该日期在本片 ≥2 个源出现"的部分。
ALTER TABLE epid_alignment_funnel_snapshot
  ADD COLUMN d_slots_total BIGINT NOT NULL DEFAULT 0 COMMENT '纯日期槽总量（M8）' AFTER l6_slot,
  ADD COLUMN d_slots_cross BIGINT NOT NULL DEFAULT 0 COMMENT '其中日期被≥2源覆盖的槽（M8）' AFTER d_slots_total;