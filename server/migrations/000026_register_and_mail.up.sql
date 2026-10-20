-- 000026 账号注册与邮件通道 (docs/design/账号注册功能设计.md):
--   users.email          注册邮箱 + 唯一约束(MySQL 唯一索引允许多 NULL, 老用户兼容)
--   users.email_verified 邮箱已验证(注册/激活链路均置 1)
--   site_config          注册模式 register_mode + SMTP 一列组(凭据与 tmdb_api_key 同范式:
--                        entity json:"-" 硬隔离, 管理端掩码读、独立端点写, 不上应用层加密)
--   register_apply       申请注册审核表(P2 启用; 刻意不存密码 —— 密码激活时才设)
ALTER TABLE users
    ADD COLUMN email          VARCHAR(190) NULL     COMMENT '注册邮箱(可空; 后台建号的老用户无邮箱)' AFTER password,
    ADD COLUMN email_verified TINYINT(1)   NOT NULL DEFAULT 0 COMMENT '邮箱已验证: 注册/激活链路置 1',
    ADD UNIQUE KEY uk_users_email(email);

-- site_config 单行(id 固定 1)。smtp_pass 凭据列注释同 tmdb_api_key 口径。
ALTER TABLE site_config
    ADD COLUMN register_mode VARCHAR(16)  NOT NULL DEFAULT 'off' COMMENT '注册模式: off 关 / open 自由注册 / apply 申请审核' AFTER hint,
    ADD COLUMN smtp_enabled  TINYINT(1)   NOT NULL DEFAULT 0  COMMENT '邮件通道总开关(被轰炸可一键停发不清凭据)' AFTER register_mode,
    ADD COLUMN smtp_host     VARCHAR(190) NOT NULL DEFAULT '' COMMENT 'SMTP 服务器' AFTER smtp_enabled,
    ADD COLUMN smtp_port     INT          NOT NULL DEFAULT 465 COMMENT 'SMTP 端口' AFTER smtp_host,
    ADD COLUMN smtp_secure   VARCHAR(8)   NOT NULL DEFAULT 'ssl' COMMENT '加密: ssl(465) / starttls(587) / none' AFTER smtp_port,
    ADD COLUMN smtp_user     VARCHAR(190) NOT NULL DEFAULT '' COMMENT 'SMTP 账号(邮箱地址)' AFTER smtp_secure,
    ADD COLUMN smtp_pass     VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'SMTP 鉴权信息(授权码/应用专用密码); json:"-" 不出网' AFTER smtp_user,
    ADD COLUMN smtp_from_name VARCHAR(64) NOT NULL DEFAULT '' COMMENT '发件人显示名' AFTER smtp_pass;

CREATE TABLE IF NOT EXISTS register_apply (
    id            BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_name     VARCHAR(64)  NOT NULL COMMENT '申请用户名(激活时如被占用可改名)',
    email         VARCHAR(190) NOT NULL COMMENT '申请邮箱(激活邮件送达处)',
    reason        VARCHAR(500) NOT NULL DEFAULT '' COMMENT '申请人自述/邀请人, 管理员审核参考',
    source_ip     VARCHAR(45)  NOT NULL DEFAULT '' COMMENT '申请来源 IP, 识别同源批量申请',
    status        TINYINT      NOT NULL DEFAULT 0 COMMENT '0 待审 / 1 已通过 / 2 已驳回',
    reject_reason VARCHAR(255) NOT NULL DEFAULT '',
    reviewed_by   BIGINT       NULL COMMENT '审核管理员 user id',
    reviewed_at   DATETIME(3)  NULL,
    activated_at  DATETIME(3)  NULL COMMENT '激活完成(密码设好)时间',
    created_at    DATETIME(3)  NULL,
    KEY idx_apply_status_created(status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
