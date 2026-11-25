-- 000062 域名替换规则增加生效范围（主站 / 从站）（2026-11-25）
--
-- 背景：封面(vod_pic)只可能来自主站；播放地址(vod_play_url)分散在主站与各从站上，
--       不同源可能用不同播放域名，替换前必须知道规则指向哪个源。
-- 取值：scope = master | slave；source_code 仅 scope=slave 时有意义（collect_source.id，
--       空串表示"全部从站"）。历史行按 master 处理（当时的解析只覆盖主站链路）。

ALTER TABLE domain_replace_rule
  ADD COLUMN scope       VARCHAR(8)  NOT NULL DEFAULT 'master' COMMENT '生效范围: master 主站 / slave 从站' AFTER field_label,
  ADD COLUMN source_code VARCHAR(64) NOT NULL DEFAULT ''       COMMENT '从站时的采集源 id(collect_source.id), 空=全部从站' AFTER scope;