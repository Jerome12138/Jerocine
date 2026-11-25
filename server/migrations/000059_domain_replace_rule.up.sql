-- 000059 源站域名替换规则表（2026-11-25）
--
-- 背景：主站（如 lz）出「替换助手」公告，要求下游站点把库里的旧域名换成新域名。
--       实测（2026-11-25）：播放地址新采已换好，但**封面 p.lztutu.com 仍有 92%+**，
--       属存量清污而非增量问题，故需要可复用、可审计、可预览的规则驱动工具。
--
-- 设计要点：
--  - 规则与"来源"解耦：source_url 只记录出处，规则本身可独立存在（公开站的主站会换）。
--  - from_host/to_host 只存**裸 host**（不带协议/路径/端口），由解析器保证口径统一。
--  - state 全生命周期：pending(待处理) / applied(已执行) / merged(被更新的公告合并) / rejected(人工拒绝)。
--  - 唯一键 (field, from_host)：同一字段同一旧域名只能有一条有效规则（新公告覆盖并 merged 旧的）。
CREATE TABLE IF NOT EXISTS domain_replace_rule (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  field         VARCHAR(32)  NOT NULL COMMENT '目标字段: vod_pic / vod_play_url / vod_down_url',
  from_host     VARCHAR(255) NOT NULL COMMENT '旧域名(裸 host)',
  to_host       VARCHAR(255) NOT NULL COMMENT '新域名(裸 host)',
  ops_tag       VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '公告里的 ops 标记, 便于追溯',
  source_url    VARCHAR(512) NOT NULL DEFAULT '' COMMENT '公告来源页',
  announced_at  BIGINT       NOT NULL DEFAULT 0 COMMENT '公告发布时间(毫秒)',
  state         VARCHAR(16)  NOT NULL DEFAULT 'pending' COMMENT 'pending/applied/merged/rejected',
  note          VARCHAR(255) NOT NULL DEFAULT '' COMMENT '备注/拒绝原因',
  created_at    BIGINT       NOT NULL DEFAULT 0,
  updated_at    BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uk_field_from (field, from_host),
  KEY idx_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='源站域名替换公告规则(下游侧)';
