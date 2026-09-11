-- 000016: 统一采集源 id 命名 —— 裸名(huya/sb)改为 src_ 前缀, 与种子源(src_lz 等)风格一致。
-- 规则(与 manage_service.collectSourceIDRe 一致): ^[a-z][a-z0-9_]{1,31}$。
-- id 被四处以字符串引用且无外键, 改名必须同步: movie_play_source.site_id /
-- collect_failure.source_id / source_health.source_id / cron_task.source_ids(json)。
-- 全部语句幂等: 重复执行(或在新环境重放)零命中即无操作。

UPDATE collect_source    SET id = 'src_huya' WHERE id = 'huya';
UPDATE collect_source    SET id = 'src_sb'   WHERE id = 'sb';

UPDATE movie_play_source SET site_id = 'src_huya' WHERE site_id = 'huya';
UPDATE movie_play_source SET site_id = 'src_sb'   WHERE site_id = 'sb';

UPDATE collect_failure   SET source_id = 'src_huya' WHERE source_id = 'huya';
UPDATE collect_failure   SET source_id = 'src_sb'   WHERE source_id = 'sb';

UPDATE source_health     SET source_id = 'src_huya' WHERE source_id = 'huya';
UPDATE source_health     SET source_id = 'src_sb'   WHERE source_id = 'sb';

-- cron_task.source_ids 为 json 字符串数组: 逐元素 CASE 改写, 其余元素原样保留;
-- 空数组/NULL 不满足 WHERE, 不被触碰(避免 JSON_ARRAYAGG 空集返回 NULL 把空数组改写坏)。
UPDATE cron_task
SET source_ids = (
  SELECT COALESCE(
    JSON_ARRAYAGG(CASE j.v WHEN 'huya' THEN 'src_huya' WHEN 'sb' THEN 'src_sb' ELSE j.v END),
    JSON_ARRAY())
  FROM JSON_TABLE(COALESCE(source_ids, JSON_ARRAY()), '$[*]'
       COLUMNS (v VARCHAR(64) PATH '$')) AS j
)
WHERE JSON_CONTAINS(COALESCE(source_ids, JSON_ARRAY()), '"huya"')
   OR JSON_CONTAINS(COALESCE(source_ids, JSON_ARRAY()), '"sb"');
