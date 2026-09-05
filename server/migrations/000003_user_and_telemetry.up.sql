-- 000003 用户与埋点: users / user_history / user_favorite / telemetry_event / telemetry_issue_resolution
-- 说明: users 保留软删除(gorm.Model 语义, 管理员维护、极少删); 历史/收藏硬删; 埋点用 datetime(3) 便于时间窗口聚合。

-- users 用户 (软删, 自增起始 10000)
CREATE TABLE IF NOT EXISTS users (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3)  NULL,
  updated_at DATETIME(3)  NULL,
  deleted_at DATETIME(3)  NULL,
  user_name  VARCHAR(64)  NOT NULL,
  password   VARCHAR(255) NOT NULL,
  role       INT          NOT NULL DEFAULT 0,      -- 0 普通 / 1 管理
  UNIQUE KEY uk_user_name(user_name),
  KEY idx_users_deleted_at(deleted_at)
) ENGINE=InnoDB AUTO_INCREMENT=10000 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- user_history 观看历史 (硬删, user+mid 唯一)
CREATE TABLE IF NOT EXISTS user_history (
  id         BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id    BIGINT      NOT NULL,
  mid        BIGINT      NOT NULL,
  play_from  VARCHAR(64) NOT NULL DEFAULT '',
  episode    INT         NOT NULL DEFAULT 0,
  progress   INT         NOT NULL DEFAULT 0,       -- 已观看秒数
  duration   INT         NOT NULL DEFAULT 0,       -- 总时长秒数
  created_at BIGINT      NOT NULL,
  updated_at BIGINT      NOT NULL,
  UNIQUE KEY uk_user_mid_history(user_id, mid),
  KEY idx_user_updated(user_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- user_favorite 收藏 (硬删, user+mid 唯一)
CREATE TABLE IF NOT EXISTS user_favorite (
  id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id    BIGINT NOT NULL,
  mid        BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  UNIQUE KEY uk_user_mid_fav(user_id, mid),
  KEY idx_user_created(user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- telemetry_event 埋点事件 (server_ts datetime(3) + 多窗口索引)
CREATE TABLE IF NOT EXISTS telemetry_event (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  category   VARCHAR(64)  NOT NULL DEFAULT '',
  path       VARCHAR(255) NOT NULL DEFAULT '',
  label      VARCHAR(255) NOT NULL DEFAULT '',
  value      DOUBLE       NOT NULL DEFAULT 0,
  duration   INT          NOT NULL DEFAULT 0,
  user_id    BIGINT       NULL,
  session_id VARCHAR(64)  NOT NULL DEFAULT '',
  ip         VARCHAR(64)  NOT NULL DEFAULT '',
  ua         VARCHAR(512) NOT NULL DEFAULT '',
  extra      JSON,
  client_ts  BIGINT       NOT NULL DEFAULT 0,
  server_ts  DATETIME(3)  NOT NULL,
  KEY idx_type_server_ts(category, server_ts),
  KEY idx_user_server_ts(user_id, server_ts),
  KEY idx_path_server_ts(path, server_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- telemetry_issue_resolution 问题闭环 (key=sha1(category|path|label))
CREATE TABLE IF NOT EXISTS telemetry_issue_resolution (
  id                 BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  key_hash           CHAR(40)     NOT NULL,
  category           VARCHAR(64)  NOT NULL DEFAULT '',
  path               VARCHAR(255) NOT NULL DEFAULT '',
  label              VARCHAR(255) NOT NULL DEFAULT '',
  resolved_commit_id VARCHAR(64)  NOT NULL DEFAULT '',
  resolved           TINYINT      NOT NULL DEFAULT 1,
  reopened_count     INT          NOT NULL DEFAULT 0,
  created_at         BIGINT       NOT NULL,
  updated_at         BIGINT       NOT NULL,
  UNIQUE KEY uk_key_hash(key_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
