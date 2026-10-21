-- 000027 出站邮件全局日配额移入后台配置(docs/design/账号注册功能设计.md §5)。
-- 原为 env MAIL_DAILY_QUOTA: 一个运营阈值放部署级变量, 微服务部署时最容易漏配,
-- 且与"邮件/注册配置统一在后台热生效"的口径不一致 ⇒ 改存 site_config,
-- 后台「系统 → 邮件与注册」直接改, 无需重启。
ALTER TABLE site_config
    ADD COLUMN mail_daily_quota INT NOT NULL DEFAULT 50
        COMMENT '出站邮件全局日配额(码/激活/驳回/重置/测试统一计数, 默认 50)' AFTER smtp_from_name;
