DROP TABLE IF EXISTS register_apply;
ALTER TABLE site_config
    DROP COLUMN register_mode,
    DROP COLUMN smtp_enabled,
    DROP COLUMN smtp_host,
    DROP COLUMN smtp_port,
    DROP COLUMN smtp_secure,
    DROP COLUMN smtp_user,
    DROP COLUMN smtp_pass,
    DROP COLUMN smtp_from_name;
ALTER TABLE users DROP INDEX uk_users_email;
ALTER TABLE users
    DROP COLUMN email_verified,
    DROP COLUMN email;
