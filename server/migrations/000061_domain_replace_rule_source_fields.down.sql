ALTER TABLE domain_replace_rule
  DROP COLUMN field_label,
  DROP COLUMN source_key,
  DROP COLUMN rule_type,
  DROP COLUMN source_note,
  ADD COLUMN ops_tag VARCHAR(64) NOT NULL DEFAULT '' COMMENT '公告里的 ops 标记, 便于追溯' AFTER to_host;
