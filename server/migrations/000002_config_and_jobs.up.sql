-- 000002 配置与任务: collect_source / cron_task / site_config / app_version / files

-- collect_source 采集源 (id 为站点字符串标识)
CREATE TABLE IF NOT EXISTS collect_source (
  id            VARCHAR(32)  NOT NULL PRIMARY KEY,
  name          VARCHAR(128) NOT NULL,
  uri           VARCHAR(512) NOT NULL,
  result_model  TINYINT      NOT NULL DEFAULT 0,   -- 0 json / 1 xml
  grade         TINYINT      NOT NULL DEFAULT 1,   -- 0 主站 / 1 附属
  sync_pictures TINYINT      NOT NULL DEFAULT 0,
  collect_type  INT          NOT NULL DEFAULT 0,
  grade_score   INT          NOT NULL DEFAULT 0,
  interval_ms   INT          NOT NULL DEFAULT 0,
  state         TINYINT      NOT NULL DEFAULT 0,   -- 0 启用 / 1 停用
  created_at    BIGINT       NOT NULL,
  updated_at    BIGINT       NOT NULL,
  UNIQUE KEY uk_uri(uri)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- cron_task 采集定时任务
CREATE TABLE IF NOT EXISTS cron_task (
  id          BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_ids  JSON,
  spec        VARCHAR(64) NOT NULL,
  time        INT         NOT NULL DEFAULT 0,
  model       TINYINT     NOT NULL DEFAULT 0,      -- 0 自动全站 / 1 指定源
  state       TINYINT     NOT NULL DEFAULT 1,      -- 0 启用 / 1 停用
  remark      VARCHAR(255) NOT NULL DEFAULT '',
  entry_id    INT         NOT NULL DEFAULT 0,
  last_run_at BIGINT      NOT NULL DEFAULT 0,
  created_at  BIGINT      NOT NULL,
  updated_at  BIGINT      NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- site_config 站点基础配置 (单行, id 固定 1)
CREATE TABLE IF NOT EXISTS site_config (
  id          TINYINT      NOT NULL PRIMARY KEY DEFAULT 1,
  site_name   VARCHAR(128) NOT NULL DEFAULT '',
  domain      VARCHAR(255) NOT NULL DEFAULT '',
  logo        VARCHAR(512) NOT NULL DEFAULT '',
  keyword     VARCHAR(255) NOT NULL DEFAULT '',
  description VARCHAR(512) NOT NULL DEFAULT '',
  state       TINYINT      NOT NULL DEFAULT 0,
  hint        VARCHAR(255) NOT NULL DEFAULT '',
  updated_at  BIGINT       NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- app_version APK 自升级版本
CREATE TABLE IF NOT EXISTS app_version (
  id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  channel      TINYINT      NOT NULL DEFAULT 0,    -- 0 stable / 1 beta
  version_code INT          NOT NULL,
  version_name VARCHAR(32)  NOT NULL,
  apk_url      VARCHAR(512) NOT NULL DEFAULT '',
  changelog    TEXT,
  `force`      TINYINT      NOT NULL DEFAULT 0,
  whitelist    JSON,
  created_at   BIGINT       NOT NULL,
  KEY idx_channel_code(channel, version_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- files 图库/二进制元数据 (BlobStore 管二进制, 此表只管元数据)
CREATE TABLE IF NOT EXISTS files (
  id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  link         VARCHAR(512) NOT NULL DEFAULT '',
  object_key   VARCHAR(255) NOT NULL DEFAULT '',
  uid          INT          NOT NULL DEFAULT 0,
  relevance_id BIGINT       NOT NULL DEFAULT 0,
  type         INT          NOT NULL DEFAULT 0,    -- 0 影片封面 / 1 用户头像
  fid          VARCHAR(128) NOT NULL DEFAULT '',
  file_type    VARCHAR(32)  NOT NULL DEFAULT '',
  created_at   BIGINT       NOT NULL,
  KEY idx_relevance(relevance_id, type)            -- 根治封面回填全表扫
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
