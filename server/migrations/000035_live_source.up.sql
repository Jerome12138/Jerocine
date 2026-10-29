-- 000035 直播源（m3u 播放列表）。
-- 背景: 直播页此前不存在; 多路直播源(每条一个 m3u 地址, 内含频道与分组)需要独立配置 ——
--   后台可增删改/启停/排序, 公开端点按需解析 m3u 出频道列表(带缓存, 源地址变更即自然换键)。
CREATE TABLE IF NOT EXISTS live_source (
  id         BIGINT        NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(64)   NOT NULL DEFAULT '',              -- 显示名
  url        VARCHAR(1024) NOT NULL DEFAULT '',              -- m3u 播放列表地址
  remark     VARCHAR(255)  NOT NULL DEFAULT '',              -- 备注
  sort       INT           NOT NULL DEFAULT 0,               -- 越小越靠前
  state      TINYINT       NOT NULL DEFAULT 0,               -- 0 启用 / 1 停用(与 banner 同口径)
  created_at BIGINT        NOT NULL,
  updated_at BIGINT        NOT NULL,
  KEY idx_state_sort(state, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- 回滚 000035: 移除直播源配置表。
DROP TABLE IF EXISTS live_source;
