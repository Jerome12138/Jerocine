-- 000016 down: 回滚统一命名(src_huya/src_sb → huya/sb)。与 up 同样幂等。
-- 注意: 回滚前须确保没有新增的 src_huya/src_sb 之外的引用方; cron_task 仅回写两元素。

UPDATE collect_source    SET id = 'huya' WHERE id = 'src_huya';
UPDATE collect_source    SET id = 'sb'   WHERE id = 'src_sb';

UPDATE movie_play_source SET site_id = 'huya' WHERE site_id = 'src_huya';
UPDATE movie_play_source SET site_id = 'sb'   WHERE site_id = 'src_sb';

UPDATE collect_failure   SET source_id = 'huya' WHERE source_id = 'src_huya';
UPDATE collect_failure   SET source_id = 'sb'   WHERE source_id = 'src_sb';

UPDATE source_health     SET source_id = 'huya' WHERE source_id = 'src_huya';
UPDATE source_health     SET source_id = 'sb'   WHERE source_id = 'src_sb';

UPDATE cron_task
SET source_ids = (
  SELECT COALESCE(
    JSON_ARRAYAGG(CASE j.v WHEN 'src_huya' THEN 'huya' WHEN 'src_sb' THEN 'sb' ELSE j.v END),
    JSON_ARRAY())
  FROM JSON_TABLE(COALESCE(source_ids, JSON_ARRAY()), '$[*]'
       COLUMNS (v VARCHAR(64) PATH '$')) AS j
)
WHERE JSON_CONTAINS(COALESCE(source_ids, JSON_ARRAY()), '"src_huya"')
   OR JSON_CONTAINS(COALESCE(source_ids, JSON_ARRAY()), '"src_sb"');
