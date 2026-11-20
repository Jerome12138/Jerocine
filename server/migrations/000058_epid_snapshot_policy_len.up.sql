-- 000058 漏斗快照表 policy_version 列宽扩展（2026-11-20）
--
-- 背景：000053 建 `policy_version` 时为 VARCHAR(32)，旧版本串
--       `L0-L7+anchors+noMixed@2026-11-19`（32 字符）恰好装满；2026-11-20 版本串递增为
--       `L0-L7+anchors+noMixed+rangeExcl@2026-11-20`（42 字符）后，快照写入
--       Error 1406 "Data too long" ⇒ 事务回滚、GET 恒返回旧快照（每次刷新计算 ~5-9min 白跑）。
-- 方案：扩到 VARCHAR(64)，为后续版本串递增留余量；MODIFY 保留 NOT NULL 与原注释语义。
ALTER TABLE epid_alignment_funnel_snapshot
  MODIFY policy_version VARCHAR(64) NOT NULL COMMENT '口径版本（L0~L6 语义变更时递增）';
