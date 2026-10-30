-- 000036 直播频道管理（覆盖层）。
-- 背景: 频道由 m3u 自动解析, 无法逐频道管理。本表对解析结果做**覆盖层**:
--   - hidden: 前台隐藏(解析频道"删除"即置位; 不物理删, 源重解析后仍按覆盖层生效)
--   - display_name: 显示名覆盖(空 = 用解析名)
--   - is_custom = 1: 后台手工添加的自定义频道(url 不依赖 m3u 解析, 物理删可用)
-- CORS/活性探源结果属易失数据, 存 Redis(v1:live:health:*)不入库。
CREATE TABLE IF NOT EXISTS live_channel_state (
  id           BIGINT        NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_id    BIGINT        NOT NULL,                       -- 所属直播源
  url          VARCHAR(768)  NOT NULL DEFAULT '',            -- 频道地址(与 m3u 解析条目按 url 对齐)
  name         VARCHAR(255)  NOT NULL DEFAULT '',            -- 自定义频道名(仅 is_custom=1 用)
  group_name   VARCHAR(128)  NOT NULL DEFAULT '',            -- 自定义频道分组(仅 is_custom=1 用)
  hidden       TINYINT       NOT NULL DEFAULT 0,             -- 1 = 前台隐藏
  display_name VARCHAR(255)  NOT NULL DEFAULT '',            -- 显示名覆盖(空 = 用解析名)
  is_custom    TINYINT       NOT NULL DEFAULT 0,             -- 1 = 后台手工添加
  created_at   BIGINT        NOT NULL,
  updated_at   BIGINT        NOT NULL,
  UNIQUE KEY uk_src_url (source_id, url(191)),
  KEY idx_src_hidden (source_id, hidden)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
