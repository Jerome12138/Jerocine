ALTER TABLE site_config
    DROP COLUMN auth_token_ttl_hours,
    DROP COLUMN max_device_login,
    DROP COLUMN spider_max_goroutine;
