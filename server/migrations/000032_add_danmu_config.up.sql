-- 弹幕服务后台配置（默认不启用；可选开放平台存 JSON 数组，空=全关）
ALTER TABLE site_config
    ADD COLUMN danmu_enabled TINYINT NOT NULL DEFAULT 0 COMMENT '弹幕总开关: 0 关闭(默认) / 1 启用',
    ADD COLUMN danmu_providers JSON NULL COMMENT '启用的弹幕平台白名单, 如 ["bilibili","tencent","iqiyi","youku"]; 空=未开放任何平台';
