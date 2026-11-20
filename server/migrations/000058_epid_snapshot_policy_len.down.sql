-- 000058 down：回滚 policy_version 列宽至 000053 原值 VARCHAR(32)。
-- ⚠️ 仅当库中不再写入 >32 字符的版本串（即回退到 `L0-L7+anchors+noMixed@2026-11-19`
--     及更短的口径串）时才可执行——否则快照写入将再次 Error 1406。
ALTER TABLE epid_alignment_funnel_snapshot
  MODIFY policy_version VARCHAR(32) NOT NULL COMMENT '口径版本（L0~L6 语义变更时递增）';
