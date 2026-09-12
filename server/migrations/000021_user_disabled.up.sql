-- 用户禁用标记: 1=禁用(禁止登录并踢掉全部在线 token), 0=正常。
-- 管理后台用户管理用; 禁用动作同时调 cache.ClearUserTokens 即时生效。
ALTER TABLE users ADD COLUMN disabled TINYINT NOT NULL DEFAULT 0;
