-- 000014 首页轮播 Banner。
-- 背景: 首页那张大图此前是拿"第一个分类区块的 hot/latest"临场拼出来的, 没有独立配置 ——
--   既挑不了、排不了、也停不掉, 而且采集源只给竖海报, 宽幅位只能靠模糊放大兜底, 观感差。
-- 独立成表后: 后台可增删改排序, 指定横图(宽幅主视觉)与竖图(窄屏兜底), 支持跳站内/外链。
CREATE TABLE IF NOT EXISTS banner (
  id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  title      VARCHAR(128) NOT NULL DEFAULT '',
  subtitle   VARCHAR(255) NOT NULL DEFAULT '',
  image      VARCHAR(512) NOT NULL DEFAULT '',   -- 横图(宽幅主视觉); 空则回退 poster
  poster     VARCHAR(512) NOT NULL DEFAULT '',   -- 竖图(窄屏/兜底); 可空
  mid        BIGINT       NOT NULL DEFAULT 0,    -- 关联影片(跳详情); 0=不关联
  link       VARCHAR(512) NOT NULL DEFAULT '',   -- 自定义跳转(站内路径或外链), 优先于 mid
  sort       INT          NOT NULL DEFAULT 0,    -- 越小越靠前
  state      TINYINT      NOT NULL DEFAULT 0,    -- 0 启用 / 1 停用
  start_at   BIGINT       NOT NULL DEFAULT 0,    -- 生效起(ms); 0=不限
  end_at     BIGINT       NOT NULL DEFAULT 0,    -- 生效止(ms); 0=不限
  created_at BIGINT       NOT NULL,
  updated_at BIGINT       NOT NULL,
  KEY idx_state_sort(state, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
