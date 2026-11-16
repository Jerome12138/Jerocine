-- P0-3（2026-11-10）：观看历史加「集身份」epid 列。
--
-- 只加列：现有 play_from / episode / progress / duration **一个不动**；
-- 默认空串 ⇒ 旧数据与老客户端（不发 epid）行为完全不变（渲染回退 play_from + episode 下标）。
-- 见 docs/方案-集身份epid落地路线与弹幕准确性-2026-11-09.md 的 P0-3 与 §3.1.9。
-- 长度 160 的依据：实测集名最长 91 字符，epid 是 5 段序列化串（必 ≤ 集名上界）。
ALTER TABLE user_history ADD COLUMN epid VARCHAR(160) NOT NULL DEFAULT '' AFTER episode;
