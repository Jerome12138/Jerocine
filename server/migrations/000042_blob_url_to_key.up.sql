-- 000042: 本地 blob 资源由「带站点前缀的 URL」改为「裸 objectKey」存储 (2026-11-09, 方案2)。
--
-- 背景: 图片/APK 的访问前缀此前被写进每一行数据(如 /api/upload/backdrop/1.jpg), 导致
--   "换对外路径 / 换独立 CDN 域名"必须改写全表。改为只存裸 key(backdrop/1.jpg)后,
--   出参时由后端按配置的基址(BLOB_BASE_URL)拼完整 URL(见 internal/bloburl), 换基址零迁移、即时生效。
--
-- 只改数据, 不动文件(文件按 objectKey 落盘, 与前缀无关)。
-- 兼容两类历史前缀: /api/upload/(旧) 与 /filestore/(同一改造的中间态)。
-- 外链守卫: WHERE 必须括号包住两个前缀谓词再 AND NOT LIKE '%://%' —— 不括号的话
--   AND 只绑右半边, "https://host/api/upload/a.jpg" 这类恰含前缀子串的外链仍会被
--   REPLACE 剥成裂图(2026-11-10 评审 Low; 与 down.sql 的 NOT LIKE 守卫对齐,
--   Go 侧 bloburl.ToKey 对含 :// 的串同样原样放行)。
-- 覆盖所有可能写入 blob URL 的列: banner.image/poster、movie.cover/backdrop、
--   movie_search.cover/backdrop、files.link、app_version.apk_url、site_config.logo。

UPDATE banner       SET image    = REPLACE(REPLACE(image,    '/api/upload/', ''), '/filestore/', '') WHERE (image    LIKE '%/api/upload/%' OR image    LIKE '%/filestore/%') AND image    NOT LIKE '%://%';
UPDATE banner       SET poster   = REPLACE(REPLACE(poster,   '/api/upload/', ''), '/filestore/', '') WHERE (poster   LIKE '%/api/upload/%' OR poster   LIKE '%/filestore/%') AND poster   NOT LIKE '%://%';
UPDATE movie        SET cover    = REPLACE(REPLACE(cover,    '/api/upload/', ''), '/filestore/', '') WHERE (cover    LIKE '%/api/upload/%' OR cover    LIKE '%/filestore/%') AND cover    NOT LIKE '%://%';
UPDATE movie        SET backdrop = REPLACE(REPLACE(backdrop, '/api/upload/', ''), '/filestore/', '') WHERE (backdrop LIKE '%/api/upload/%' OR backdrop LIKE '%/filestore/%') AND backdrop NOT LIKE '%://%';
UPDATE movie_search SET cover    = REPLACE(REPLACE(cover,    '/api/upload/', ''), '/filestore/', '') WHERE (cover    LIKE '%/api/upload/%' OR cover    LIKE '%/filestore/%') AND cover    NOT LIKE '%://%';
UPDATE movie_search SET backdrop = REPLACE(REPLACE(backdrop, '/api/upload/', ''), '/filestore/', '') WHERE (backdrop LIKE '%/api/upload/%' OR backdrop LIKE '%/filestore/%') AND backdrop NOT LIKE '%://%';
UPDATE files        SET link     = REPLACE(REPLACE(link,     '/api/upload/', ''), '/filestore/', '') WHERE (link     LIKE '%/api/upload/%' OR link     LIKE '%/filestore/%') AND link     NOT LIKE '%://%';
UPDATE app_version  SET apk_url  = REPLACE(REPLACE(apk_url,  '/api/upload/', ''), '/filestore/', '') WHERE (apk_url  LIKE '%/api/upload/%' OR apk_url  LIKE '%/filestore/%') AND apk_url  NOT LIKE '%://%';
UPDATE site_config  SET logo     = REPLACE(REPLACE(logo,     '/api/upload/', ''), '/filestore/', '') WHERE (logo     LIKE '%/api/upload/%' OR logo     LIKE '%/filestore/%') AND logo     NOT LIKE '%://%';
