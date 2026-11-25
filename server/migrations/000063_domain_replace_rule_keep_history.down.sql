-- 回滚 000063：唯一键从 (field, from_host, to_host) 退回 (field, from_host)。
--
-- 风险：新版本允许同一旧域名保留多条**历史目标**行，直接加回旧唯一键会冲突并中断回滚。
-- 处理：先把"非最新"的历史行整行归档到 domain_replace_rule_hist_bak（不删数据），
--       再从主表移除，最后换回旧唯一键。回滚后如需保留历史，从备份表按 id 取回即可。

CREATE TABLE IF NOT EXISTS domain_replace_rule_hist_bak LIKE domain_replace_rule;

INSERT INTO domain_replace_rule_hist_bak
SELECT * FROM domain_replace_rule
 WHERE id NOT IN (
   SELECT id FROM (
     SELECT MAX(id) AS id FROM domain_replace_rule GROUP BY field, from_host
   ) t
 );

DELETE FROM domain_replace_rule
 WHERE id NOT IN (
   SELECT id FROM (
     SELECT MAX(id) AS id FROM domain_replace_rule GROUP BY field, from_host
   ) t
 );

ALTER TABLE domain_replace_rule
  DROP INDEX uk_field_from_to,
  ADD UNIQUE KEY uk_field_from (field, from_host);