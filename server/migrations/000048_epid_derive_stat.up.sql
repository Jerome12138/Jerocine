-- 000048：派生进度**写侧计数器** epid_derive_stat（单行表）。
--
-- 背景（2026-11-11 线上实测）：
--   「集身份（epid）派生」卡片在**任务进行中**必须显示"跑到哪了 / 还缺多少"。原实现用精确计数：
--     · `SELECT COUNT(*) FROM movie_episode_epid`              实测 **19.2s**（预算 3s）✗
--     · `SELECT COUNT(DISTINCT src_row_id) FROM movie_episode_epid` 实测 **8.6s**（预算 5s）✗
--     · `SELECT COUNT(*) FROM movie_play_source`                实测 **5.3s**（表里挂着 1.1GB 的 episodes_json 巨列）
--   派生表涨到 **590 万行**后三项全部撞穿各自超时 ⇒ 按"每项独立超时 + 全项容错"被降级 ⇒ 卡片显示 0/0 ✗
--
-- 解法：把"全表计数"换成**增量维护的计数器**（O(1) 读）：
--   · 写侧唯一入口 `ReplaceForSourceRows` 在同一事务内按"插入数 − 删除数"更新（原子 ✓）
--   · `TruncateAll`（清空重建）重置为 0
--   · `DeleteBySrcRowIds`（孤儿清理 / 抖动回滚）按实删条数扣减
--   ⇒ 读侧一次主键点查，成本与表规模**无关** ✓
--
-- 本迁移末尾用**一次性精确计数**做基线校准（此刻派生表约 590 万行，两条 COUNT 合计约 20~40s；
-- 部署时后端正在重启、无派生任务在跑 ⇒ 不会与写侧并发；空库则瞬时完成）。
CREATE TABLE IF NOT EXISTS epid_derive_stat (
  id               TINYINT UNSIGNED NOT NULL COMMENT '恒为 1（单行表）',
  derived_rows     BIGINT           NOT NULL DEFAULT 0 COMMENT 'movie_episode_epid 当前行数（写侧增量维护）',
  derived_src_rows BIGINT           NOT NULL DEFAULT 0 COMMENT '已派生过的源行数 = COUNT(DISTINCT src_row_id)（写侧增量维护）',
  updated_at       BIGINT           NOT NULL DEFAULT 0 COMMENT '最后一次维护时间（毫秒）',
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='epid 派生进度计数器（单行；O(1) 读数，写侧同事务维护）';

-- ① 先确保单行存在（否则写侧 UPDATE 不命中任何行）
INSERT INTO epid_derive_stat (id, derived_rows, derived_src_rows, updated_at)
VALUES (1, 0, 0, UNIX_TIMESTAMP() * 1000)
ON DUPLICATE KEY UPDATE id = id;

-- ② 基线校准（一次性精确计数；此后由写侧增量维护）
UPDATE epid_derive_stat
SET derived_rows     = (SELECT COUNT(*) FROM movie_episode_epid),
    derived_src_rows = (SELECT COUNT(DISTINCT src_row_id) FROM movie_episode_epid),
    updated_at       = UNIX_TIMESTAMP() * 1000
WHERE id = 1;
