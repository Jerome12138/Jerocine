-- 000038 直播顶栏入口开关(非敏感展示开关, 随公开 GET /config/site 直接下发)。
-- 语义: 有启用直播源的前提下, web/TV 顶栏导航是否显示"直播"。
-- 侧边栏/移动抽屉固定显示(不消费此列); 无源时前端按 /live/sources 空列表隐藏一切入口。
ALTER TABLE site_config
    ADD COLUMN live_nav_enabled TINYINT NOT NULL DEFAULT 1 COMMENT '直播顶栏入口: 1 显示(默认) / 0 隐藏';
