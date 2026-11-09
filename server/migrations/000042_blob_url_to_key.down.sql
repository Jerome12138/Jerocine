-- 000042 down: 裸 key → /api/upload/<key>(回退到改造前的旧前缀存储)。
--
-- ⚠ 仅在**完整回退**(连同后端 bloburl 逻辑一起退)时使用。判定: 非空、无 "://"(非外链)、不以 "/" 开头
--   的值为裸 key → 加回旧前缀; 外链与已是根相对的值原样。

UPDATE banner       SET image    = CASE WHEN image    <> '' AND image    NOT LIKE '%://%' AND image    NOT LIKE '/%' THEN CONCAT('/api/upload/', image)    ELSE image    END WHERE image    <> '';
UPDATE banner       SET poster   = CASE WHEN poster   <> '' AND poster   NOT LIKE '%://%' AND poster   NOT LIKE '/%' THEN CONCAT('/api/upload/', poster)   ELSE poster   END WHERE poster   <> '';
UPDATE movie        SET cover    = CASE WHEN cover    <> '' AND cover    NOT LIKE '%://%' AND cover    NOT LIKE '/%' THEN CONCAT('/api/upload/', cover)    ELSE cover    END WHERE cover    <> '';
UPDATE movie        SET backdrop = CASE WHEN backdrop <> '' AND backdrop NOT LIKE '%://%' AND backdrop NOT LIKE '/%' THEN CONCAT('/api/upload/', backdrop) ELSE backdrop END WHERE backdrop <> '';
UPDATE movie_search SET cover    = CASE WHEN cover    <> '' AND cover    NOT LIKE '%://%' AND cover    NOT LIKE '/%' THEN CONCAT('/api/upload/', cover)    ELSE cover    END WHERE cover    <> '';
UPDATE movie_search SET backdrop = CASE WHEN backdrop <> '' AND backdrop NOT LIKE '%://%' AND backdrop NOT LIKE '/%' THEN CONCAT('/api/upload/', backdrop) ELSE backdrop END WHERE backdrop <> '';
UPDATE files        SET link     = CASE WHEN link     <> '' AND link     NOT LIKE '%://%' AND link     NOT LIKE '/%' THEN CONCAT('/api/upload/', link)     ELSE link     END WHERE link     <> '';
UPDATE app_version  SET apk_url  = CASE WHEN apk_url  <> '' AND apk_url  NOT LIKE '%://%' AND apk_url  NOT LIKE '/%' THEN CONCAT('/api/upload/', apk_url)  ELSE apk_url  END WHERE apk_url  <> '';
UPDATE site_config  SET logo     = CASE WHEN logo     <> '' AND logo     NOT LIKE '%://%' AND logo     NOT LIKE '/%' THEN CONCAT('/api/upload/', logo)     ELSE logo     END WHERE logo     <> '';
