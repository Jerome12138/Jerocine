-- 000049：epid 源头校正**裁定台账** movie_epid_override（epid 源头校正层 · 第②段）。
--
-- 背景：
--   派生表 movie_episode_epid 单向派生自 movie_play_source.episodes_json，anchor/bridge 靠
--   "池"推算期号（约 0.84% 行），个别推错且规则修不动（源特有个例）。这类"已经判过"的结论
--   需要一个台账记录，而不是反复重算、反复人看：
--     · 只放"已判过"的结论；待定项留在离线清单，不入库；
--     · 大表 movie_episode_epid **零新增列** —— 行内只放可重算状态，裁定放外部台账 ⇒
--       增量 DELETE+INSERT 与 TRUNCATE 全量重建两条路径都**一行不用改**，校正永不丢；
--     · 应用前做**三元组校验** (src_row_id, key_raw_name, key_epid)：全对得上才套用；
--       对不上 ⇒ 标 stale 重回清单（宁缺勿错）。
--   状态机：pending → active（生效）；旁支 stale（源/规则版本变）/ obsolete（规则已能算对）/
--   revoked（人工撤销）/ orphan（源行被删）。
--
-- 索引：
--   uk_row_ep(src_row_id, ep_index) —— 唯一键，同一源行同一集只允许一条裁定；
--   idx_status —— 应用侧只取 status=active；
--   idx_match —— 按 match_key（片）统计/清理；全表聚合一律按 match_key 分批。
--
-- ⚠️ Collation 显式对齐 utf8mb4_unicode_ci（与源表 movie_play_source 及派生表一致；
-- 000044 未显式指定、000045 整表 CONVERT 补救的教训）：key_epid/match_key 会与
-- movie_episode_epid 的列做跨表比较（三元组校验/按片核对），不显式对齐会报
-- Illegal mix of collations。
--
-- 口径见《epid 源头校正层方案》（2026-11-12）§3.3 数据模型 / §10 迁移骨架。
CREATE TABLE IF NOT EXISTS movie_epid_override (
  id              BIGINT       NOT NULL AUTO_INCREMENT COMMENT '主键',
  src_row_id      BIGINT       NOT NULL COMMENT 'movie_play_source.id（定位线索）',
  ep_index        INT          NOT NULL COMMENT '该源内的集下标（定位线索）',
  match_key       VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '受控冗余：按片统计/清理/分批聚合',
  mid             BIGINT       NOT NULL DEFAULT 0 COMMENT '影片 id（与派生表口径一致；线索）',
  key_raw_name    VARCHAR(255) NOT NULL DEFAULT '' COMMENT '裁定时的集名 —— 防 ep_index 因重排/插集漂移后错位',
  key_epid        VARCHAR(160) NOT NULL DEFAULT '' COMMENT '裁定时的规则 epid —— 三元组校验第二键，防错位',
  verdict         VARCHAR(16)  NOT NULL DEFAULT 'corrected' COMMENT '裁定结论：corrected=要改 | kept=判定不必改（闭嘴）',
  new_epid        VARCHAR(160) NOT NULL DEFAULT '' COMMENT 'verdict=corrected 时的目标 epid（5 段）',
  provenance      VARCHAR(16)  NOT NULL DEFAULT 'agent' COMMENT '裁定来源：rule|external|agent|human',
  confidence      VARCHAR(16)  NOT NULL DEFAULT 'inferred' COMMENT '置信：inferred|verified',
  reason          VARCHAR(255) NOT NULL DEFAULT '' COMMENT '裁定原因',
  evidence        JSON         NULL COMMENT '证据（外部链接、抓取日期等）',
  status          VARCHAR(16)  NOT NULL DEFAULT 'pending' COMMENT 'pending|active|stale|obsolete|revoked|orphan',
  rule_version_at VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '裁定时的 P/A 规则版本（规则变更后复检）',
  created_at      BIGINT       NOT NULL DEFAULT 0 COMMENT '创建时间（毫秒）',
  verified_at     BIGINT       NOT NULL DEFAULT 0 COMMENT '人工确认时间（毫秒）',
  verified_by     VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '确认人',
  PRIMARY KEY (id),
  UNIQUE KEY uk_row_ep (src_row_id, ep_index),
  KEY idx_status (status),
  KEY idx_match (match_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='epid 源头校正裁定台账（只放已判结论；大表零新增列；应用前三元组校验）';
