-- 000048 down：删掉计数器表。
-- 回滚后读侧会退回"精确 COUNT"路径（19.2s/8.6s ⇒ 撞超时 ⇒ 卡片又显示 0/0），
-- 但派生数据本身不受影响（计数器只是派生的读数，不是真相源）✓
DROP TABLE IF EXISTS epid_derive_stat;
