-- 000033 每用户 Web 端设置快照 (账号记忆)。
--
-- 背景: Web 端的设置类数据(弹幕开关/弹幕显示配置/广告过滤/连播/线路缓存等)此前只存
--   localStorage(按浏览器记忆); 收藏/历史/跳过设置已有 /me/* 账号级接口。
--   本表提供通用 JSON blob 整存整取(前端 localStorage 镜像), 未登录仍走本地,
--   登录后双写云端 —— 新设置项按约定进 blob 即随账号记忆, 不必每次加表加列。
CREATE TABLE IF NOT EXISTS web_setting (
  user_id    BIGINT      NOT NULL PRIMARY KEY,   -- users.id, 一人一行
  data       JSON        NOT NULL,               -- key→value 快照(前端维护结构)
  updated_at BIGINT      NOT NULL DEFAULT 0      -- 毫秒时间戳(前端/服务端都好比对新旧)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
