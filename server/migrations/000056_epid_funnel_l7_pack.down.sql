-- 000056 down：回滚 L7 两列
ALTER TABLE epid_alignment_funnel_snapshot
  DROP COLUMN l7_slots,
  DROP COLUMN l7_pack_rows;