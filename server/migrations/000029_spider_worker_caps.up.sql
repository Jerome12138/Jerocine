-- 000029 采集并发硬上限进后台：主站 / 从站各自的上限原为代码常量(masterMaxWorkers=12 / slaveMaxWorkers=8)。
-- 这两个值是"写库死锁"与"被源站限流"之间的经验折中，不同库/不同源站会变，
-- 而且和 spider_max_goroutine 一起决定实际并发 ⇒ 一起放后台，避免改一个还得改代码重新发版。
ALTER TABLE site_config
    ADD COLUMN spider_master_max_workers INT NOT NULL DEFAULT 12 COMMENT '主站采集并发硬上限(主站每页写 movie+search+play 三表, 过高易 InnoDB 死锁)' AFTER spider_max_goroutine,
    ADD COLUMN spider_slave_max_workers  INT NOT NULL DEFAULT 8  COMMENT '从站采集并发硬上限(仅写播放源行, 过高易触发源站限流/403)' AFTER spider_master_max_workers;
