-- 000052：movie_play_source 补「上游元数据」两列（2026-11-14）—— 双写副本判定到 100% 的根治条件。
--
-- 背景：补充源同一部片按 `hash(NormalizeName(VodName))`（片名键 ⇒ 主份）与 `hash(VodDouBanID)`
--   （豆瓣键 ⇒ 副本）双键各存一行；判定副本**权威在写侧**，但**存量**只能反推。
--   反推实测（`docs/epid-集身份/分析-双写副本剩余判定方法-2026-11-14.md`）：
--     · `键 ∈ {hash(movie.db_id)}`                      ⇒ 判得 67.0%
--     · 再加 `hash(movie.name)`（4,015 组）与键级传播（898 组）⇒ **≈69.8%**
--     · 剩余 ~30%（53,133 组）两键都命中不了任何库内键 —— 源站片名/豆瓣 id 与我们库内口径对不上，
--       且这些源行 `mid` 全空（附属站）⇒ 任何"反推"都到顶了。
--   ⇒ 根治 = 把**上游原文**落库：有 `source_name` / `source_douban_id` 后，判定即 **100%**
--     （`hash(source_name)` 就是片名键、`hash(source_douban_id)` 就是豆瓣键），且不再依赖库内是否有该片。
--
-- ⚠️ **两列可空、刻意不回填存量**（用户 2026-11-14 定）：
--   · 只对"以后新采 / 重采到的行"生效，**不为了这两列触发全库重采**；
--   · 写入侧 upsert 用 `COALESCE(VALUES(col), col)` ⇒"有值才覆盖、空值不冲掉旧值"（宁缺勿错）；
--   · **不进**「内容未变不写」的判重列 ⇒ 不会因此把 72 万源行整体重写一遍（保住 binlog/磁盘）。
--   ⇒ 现状：存量行为 NULL；只有"内容真的变了而被写"的行才会带上（见 `play_repo.playSourceUpsertUpdates`）。
ALTER TABLE movie_play_source
    ADD COLUMN source_name VARCHAR(255) NULL COMMENT '上游片名原文(VodName)：片名键 = GenerateHashKey(source_name)；可空=存量未采(不回填)',
    ADD COLUMN source_douban_id BIGINT NULL COMMENT '上游豆瓣id(VodDouBanID)：豆瓣键 = GenerateHashKey(source_douban_id)；NULL/0=上游没有';
