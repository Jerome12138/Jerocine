-- 000063 域名替换规则：保留历史目标（唯一键加 to_host）（2026-11-25）
--
-- 用户要求：每次拉取新的**不要直接替换旧数据** ——
--   · 同 (field, from_host, to_host) 完全一致 ⇒ 跳过（不更新、不动状态）
--   · 同 (field, from_host) 但目标变了   ⇒ **新增一条**，旧行保留并把状态标 merged
-- 因此唯一键从 (field, from_host) 扩成 (field, from_host, to_host)：
-- 同一旧域名可以有多条历史目标，但同一个目标不会重复入库。
-- 已有数据在新旧两个唯一键下都唯一，迁移安全。

ALTER TABLE domain_replace_rule
  DROP INDEX uk_field_from,
  ADD UNIQUE KEY uk_field_from_to (field, from_host, to_host);