-- 000053 对齐率分层漏斗（L0~L6）异步快照表（P11，2026-11-18）
--
-- 背景：漏斗全量现算 ~9min，HTTP WriteTimeout 只有 120s ⇒ 端点不可用。
-- 方案：改为「后台刷新 + 读快照」——POST 触发一次后台计算，结果写入本表；GET 只读快照。
-- 口径版本写库（policy_version），便于 L5/L6 口径调整后与历史值对账。
CREATE TABLE IF NOT EXISTS epid_alignment_funnel_snapshot (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  computed_at    DATETIME        NOT NULL COMMENT '本次快照生成时间',
  policy_version VARCHAR(32)     NOT NULL COMMENT '口径版本（L0~L6 语义变更时递增）',
  truncated      TINYINT         NOT NULL DEFAULT 0 COMMENT '1=计算被分页上限截断（结果不完整）',
  pid            BIGINT          NOT NULL COMMENT '分类 id（-1=全部）',
  category       VARCHAR(64)     NOT NULL,
  slots_total    BIGINT          NOT NULL DEFAULT 0,
  l0_single      BIGINT          NOT NULL DEFAULT 0,
  l1_slot        BIGINT          NOT NULL DEFAULT 0,
  l2_tolerant    BIGINT          NOT NULL DEFAULT 0,
  l3_date        BIGINT          NOT NULL DEFAULT 0,
  l4_num         BIGINT          NOT NULL DEFAULT 0,
  l5_subset      BIGINT          NOT NULL DEFAULT 0,
  l6_tol         BIGINT          NOT NULL DEFAULT 0,
  mixed_diag     BIGINT          NOT NULL DEFAULT 0,
  leftover       BIGINT          NOT NULL DEFAULT 0,
  mks_total      BIGINT          NOT NULL DEFAULT 0,
  mks_aligned    BIGINT          NOT NULL DEFAULT 0,
  aligned_slots  BIGINT          NOT NULL DEFAULT 0,
  slot_pct       DECIMAL(6,2)    NOT NULL DEFAULT 0 COMMENT '集级对齐率（分层口径）',
  mk_pct         DECIMAL(6,2)    NOT NULL DEFAULT 0 COMMENT '片级对齐率（分层口径）',
  PRIMARY KEY (id),
  KEY idx_computed (computed_at),
  KEY idx_pid (pid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='对齐率分层漏斗快照（整表覆盖写：每次刷新后只保留一批）';