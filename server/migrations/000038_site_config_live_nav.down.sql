-- 回滚 000038: 移除直播顶栏入口开关列。
ALTER TABLE site_config DROP COLUMN live_nav_enabled;
