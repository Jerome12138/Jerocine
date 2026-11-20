-- 000057 L6.4 / L6.5 锚容忍（2026-11-19，用户口径）
--
-- L6.4：以 `d`（日期）为锚；L6.5：以 `n`（期号）为锚。
-- 判据：锚组内 epid 全等即可容忍；若该片存在「同一源同锚多行」（重复日期/重复期号），
--       则该锚必须由 ≥2 个源共同证明（多源一致），否则不生效。
ALTER TABLE epid_alignment_funnel_snapshot
  ADD COLUMN l6d4 BIGINT NOT NULL DEFAULT 0 COMMENT 'L6.4 日期锚容忍槽' AFTER l7_slots,
  ADD COLUMN l6n5 BIGINT NOT NULL DEFAULT 0 COMMENT 'L6.5 期号锚容忍槽' AFTER l6d4;