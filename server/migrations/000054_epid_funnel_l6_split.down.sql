-- 000054 down：回滚 L6 拆分子列（l6_tol 保留）
ALTER TABLE epid_alignment_funnel_snapshot
  DROP COLUMN l6_slot,
  DROP COLUMN l6_date,
  DROP COLUMN l6_num;