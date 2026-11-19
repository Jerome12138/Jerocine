-- 000055 down：回滚 d 覆盖率两列
ALTER TABLE epid_alignment_funnel_snapshot
  DROP COLUMN d_slots_cross,
  DROP COLUMN d_slots_total;