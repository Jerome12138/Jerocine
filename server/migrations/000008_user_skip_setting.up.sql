-- 000008 用户跳过设置: 每用户每片(mid)的片头/片尾跳过秒数, user+mid 唯一(upsert)。
CREATE TABLE IF NOT EXISTS user_skip_setting (
  id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id    BIGINT NOT NULL,
  mid        BIGINT NOT NULL,
  intro_sec  INT    NOT NULL DEFAULT 0,
  outro_sec  INT    NOT NULL DEFAULT 0,
  updated_at BIGINT NOT NULL,
  UNIQUE KEY uk_user_mid_skip(user_id, mid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
