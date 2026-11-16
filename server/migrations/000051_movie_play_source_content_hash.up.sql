-- 000051：`movie_play_source.content_hash` —— **整源内容**指纹（含 link），服务采集侧「内容未变不写」。
--
-- 背景（2026-11-13）：采集 upsert 目前**每次都重写整行**（含 `episodes_json` 巨列）⇒ 全站重采一轮
--   产生海量 binlog（实测把 binlog 撑到 12GB+，磁盘到 89%）。而绝大多数行**内容其实没变**。
--
-- 与既有 `ep_hash` 的分工（**别混**）：
--   · `ep_hash` = fnv64a(**集名序列**)，**不含 link** —— R4 实测教训：源站换 CDN / 防盗链签名轮换
--     会让 URL 天天变，若把 link 计入 ⇒ 一次重采把全部源行打脏 ⇒ **派生风暴** ✗。它只服务派生判脏。
--   · `content_hash`（本列）= fnv64a(**名\x01链接\x01 序列**)，**含 link** —— 只服务"内容未变不写"判重：
--     连 link 都没变 ⇒ 整行确实无需重写 ⇒ 跳过 UPDATE（省 binlog / 省行版本 / 省派生判脏）。
--
-- 跳过条件（见 `play_repo.UpsertBatch`）：`mid / source_vod_id / dup_copy / content_hash` **四项全等**
--   ⇒ 跳过；只要有一项不同（例如 `source_vod_id` 由 0 被回填成真值）⇒ 照常写 ✓
--   （这条保证"重采回填 vod_id / dup_copy"这类修复**不会被跳过逻辑吃掉**）。
--
-- ⚠️ 存量回填：本迁移**不回填**（留空串）。空串 ≠ 新值 ⇒ **首次重采必写并填上** ⇒ 自愈 ✓。
ALTER TABLE movie_play_source
    ADD COLUMN content_hash CHAR(16) NOT NULL DEFAULT '' COMMENT '整源内容指纹（含 link，fnv64a；采集侧「内容未变不写」判重用）';
