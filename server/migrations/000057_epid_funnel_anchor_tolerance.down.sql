-- 000057 down：回滚 L6.4/L6.5 两列
ALTER TABLE epid_alignment_funnel_snapshot
  DROP COLUMN l6n5,
  DROP COLUMN l6d4;