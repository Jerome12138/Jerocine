-- 000060 域名替换审计表（2026-11-25）
--
-- 为什么必须留审计：执行替换会**改动业务数据**（movie_search.cover）。
-- 审计表记录每行 before/after，用于抽查、对账与**回滚**（把 to 再换回 from 即可）。
-- 只记账，不加外键约束（规则可能被删/合并，但审计要长期保留）。
CREATE TABLE IF NOT EXISTS domain_replace_audit (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  rule_id    BIGINT       NOT NULL COMMENT 'domain_replace_rule.id（仅溯源，不设外键）',
  field      VARCHAR(32)  NOT NULL COMMENT '被替换的字段（当前仅 vod_pic）',
  mid        BIGINT       NOT NULL COMMENT '影片主键',
  before_val VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '替换前原值',
  after_val  VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '替换后新值',
  created_at BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_rule (rule_id),
  KEY idx_mid (mid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='域名替换审计(可回滚依据)';
