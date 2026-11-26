-- 000064 内置任务的可编辑配置（统一任务中心）
--
-- 背景：cron 定时任务的配置本来就在 cron_task 表里（启用/计划/备注），但**内置任务**
-- （域名公告拉取、豆瓣评分同步、欠账补查、榜单热度、横图补采、源健康巡检…）此前
-- 全部硬编码在代码里，既看不见也改不了。
--
-- 这张表只存"用户改过的值"，代码里的默认值仍由注册表给出：
--   读取优先级：task_setting（非空）→ 注册表默认值。
-- 两列语义分层，互不相干：
--   schedule    调度层：多久跑一次（友好文本 every:30m / daily:03:00 / weekly:1 03:00）
--   interval_ms 执行层：一次运行内相邻外部请求隔多久（防被源站拉黑，有硬下限）

CREATE TABLE IF NOT EXISTS task_setting (
  task_key    VARCHAR(64)  NOT NULL COMMENT '任务标识: builtin:douban_score / builtin:domain_fetch …',
  enabled     TINYINT(1)   NOT NULL DEFAULT 1 COMMENT '是否启用（内置任务默认不启用者由注册表给 false）',
  schedule    VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '调度层：友好文本周期，空=用默认',
  remark      VARCHAR(255) NOT NULL DEFAULT '' COMMENT '备注',
  interval_ms INT          NOT NULL DEFAULT 0 COMMENT '执行层：外部接口间隔(ms)，0=用默认，低于下限按下限执行',
  updated_at  BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (task_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='内置任务的可编辑配置（cron 任务仍用 cron_task）';
