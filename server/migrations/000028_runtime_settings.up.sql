-- 000028 运行参数进后台：登录策略 + 采集并发(docs/design/账号注册功能设计.md 决策 7 的延伸)。
-- 原为 env AUTH_TOKEN_TTL_HOURS / MAX_DEVICE_LOGIN / MAX_GOROUTINE：都是运营阈值，
-- 放部署级变量既容易漏配(compose 显式列表 + bare heredoc 两处)，又需要重启才生效。
ALTER TABLE site_config
    ADD COLUMN auth_token_ttl_hours  INT NOT NULL DEFAULT 168 COMMENT '登录态有效期(小时, 默认 168=7天; 只影响新签发 token)' AFTER mail_daily_quota,
    ADD COLUMN max_device_login      INT NOT NULL DEFAULT 5   COMMENT '单账号同时在线设备数(超出踢最旧, 下次登录时生效)' AFTER auth_token_ttl_hours,
    ADD COLUMN spider_max_goroutine  INT NOT NULL DEFAULT 32  COMMENT '采集并发上限(主站另有 12 / 从站 8 的硬上限)' AFTER max_device_login;
