-- 站点配置新增「图床域名链」: 逗号分隔, 第一个为主图床, 其余为备用域;
-- 前端封面 onerror 后在链内自动切下一个域(图床家族常换域名, 如 lztutu↔lzipic)。
ALTER TABLE site_config
  ADD COLUMN img_fallback_hosts VARCHAR(500) NOT NULL DEFAULT '' AFTER live_nav_enabled;
