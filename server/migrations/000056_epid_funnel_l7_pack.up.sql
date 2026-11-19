-- 000056 L7 特殊数据剔除层（2026-11-19，用户口径"子集法"）
--
-- 语义：槽内剔除 `s=合集`（归一后）的派生行；空集视为任何集合的子集 ⇒ 该槽判对齐。
--       仅当槽内还存在"可比较行"（非合集行）时剔除；整槽全为合集行 ⇒ 不剔（保持原判）。
-- 用途：让"只有合集"的源与"有正常集"的源能算对齐（实测可翻转 156 片）。
ALTER TABLE epid_alignment_funnel_snapshot
  ADD COLUMN l7_pack_rows BIGINT NOT NULL DEFAULT 0 COMMENT 'L7 剔除的合集行数' AFTER d_slots_cross,
  ADD COLUMN l7_slots     BIGINT NOT NULL DEFAULT 0 COMMENT 'L7 发生剔除的槽数' AFTER l7_pack_rows;