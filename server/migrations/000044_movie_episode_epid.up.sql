-- P1-1（2026-11-10）：集身份 epid 落库（第一批：变更哨兵 + 派生表）。
--
-- ① movie_play_source.ep_hash：源行集内容的 16 位十六进制指纹（fnv64a of episodes_json 规范化序列）。
--    派生任务**只比它**就知道"该源行的集内容变没变"，**不回表读 1GB 巨列**（F-05：写侧必须同步回填）。
-- ② movie_episode_epid：派生表，一集一行（实测 ≈647.7 万行 / 估算 1.5~2GB）。
--    索引按实测收窄：idx_match_epid(match_key, epid) 服务"同片同集"（index-only）；
--    idx_epid(epid) 服务跨片/离线聚合。**不冗余 link**（大字段 + 最热变动 ⇒ 只留 key 级冗余）。
--
-- 口径见 docs/方案-集身份epid落地路线与弹幕准确性-2026-11-09.md（v5.0 §5.2）。
ALTER TABLE movie_play_source ADD COLUMN ep_hash CHAR(16) NOT NULL DEFAULT '' AFTER episodes_json;

CREATE TABLE IF NOT EXISTS movie_episode_epid (
  id          BIGINT       NOT NULL AUTO_INCREMENT,
  src_row_id  BIGINT       NOT NULL COMMENT 'movie_play_source.id',
  mid         BIGINT       NULL COMMENT '与源行一致；附属源行为 NULL（靠 match_key 关联）',
  match_key   VARCHAR(64)  NOT NULL COMMENT '受控冗余：免 JOIN 巨表（性能铁律 1）',
  site_id     VARCHAR(64)  NOT NULL,
  play_from   VARCHAR(64)  NOT NULL,
  ep_index    INT          NOT NULL COMMENT '该源内的集下标',
  epid        VARCHAR(160) NOT NULL COMMENT '5 段身份串；**空串 = 不产身份**（L6）',
  from_src    VARCHAR(16)  NOT NULL COMMENT '期号来源：self|bridge|anchor|none（P1-3 的 from=）',
  raw_name    VARCHAR(255) NOT NULL COMMENT '原始集名（排查用）',
  computed_at BIGINT       NOT NULL COMMENT '派生时间（**不与源行 updated_at 同名**，避免歧义）',
  PRIMARY KEY (id),
  UNIQUE KEY uk_row_ep (src_row_id, ep_index),
  KEY idx_match_epid (match_key, epid),
  KEY idx_epid (epid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='集身份派生表（P1-1；单向派生自 movie_play_source.episodes_json）';
