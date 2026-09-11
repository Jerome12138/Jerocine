-- 000018 TMDB API Key 收编进站点配置: 管理后台可维护, 免登服务器改 .env。
-- 明文存 DB(与原 .env 同级安全面, 后台本就需登录); 公开接口靠 entity json:"-" 隔离, 永不出网。
-- 优先级: 后台 DB 值 > 启动 env TMDB_API_KEY(保留作兜底/首次部署便利)。
ALTER TABLE site_config
    ADD COLUMN tmdb_api_key VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'TMDB 凭据: v3 key 或 v4 read token; 空=横图回填关闭' AFTER hint;
