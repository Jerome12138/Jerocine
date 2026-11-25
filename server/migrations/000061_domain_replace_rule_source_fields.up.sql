-- 000061 域名替换规则补齐来源与备注（2026-11-25）
--
-- 背景：源站页面在 `data-rep-data` 里内嵌结构化 JSON，字段比页面文本完整。我们**只取对我们有用的**：
--   来源(source_key/source_url) · 公告时间 · 分类(type) · 目标列(field/label) · 备注(note/ops)。
-- 明确**不存**源站自用字段（executable 能否执行 / applied 源站已执行 / warning / published_target /
-- 源站 rule.id / ops_tag 源站发布标记）：那是给站长看的展示状态，对下游没有决策价值。
--
-- 注意：ops_tag 列**暂不删除**。迁移跑在容器重建之前，这几秒内旧进程仍会 SELECT 该列，
-- 一旦 DROP 会让旧进程报 "Unknown column"。本版本只是不再读写它，等下一版确认没有旧进程
-- 在跑之后再单独加一个迁移清理。
--
-- 早期实现只存 field/from/to（文本猜测），导致公告日期全为 0、也看不出这条是封面还是播放。

ALTER TABLE domain_replace_rule
  ADD COLUMN field_label VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '目标列中文名: 视频封面 / 视频播放 / 视频下载' AFTER field,
  ADD COLUMN source_key  VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '来源站标识: lz / manual …' AFTER field_label,
  ADD COLUMN rule_type   VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '公告分类: 视频封面替换 / 视频播放地址 / 视频下载地址' AFTER source_key,
  ADD COLUMN source_note VARCHAR(255) NOT NULL DEFAULT '' COMMENT '源站备注(已剥掉 [ops:xxx])' AFTER source_url;