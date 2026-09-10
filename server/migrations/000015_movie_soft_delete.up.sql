-- 影片软删除: 用删除时间戳(毫秒)标记, 0 = 未删。
-- 软删而非物理删的原因: 采集源随时可能把同一部片再推一遍, 物理删早晚会被"长回来";
-- 用时间戳标记后, 公开读路径一律带 deleted_at = 0, 后台可随时恢复, 采集重推也覆盖不掉标记。
-- 两表同时加列: movie 是删除状态的唯一真相源, movie_search 是它的读模型镜像(列表/检索都走这张表)。

ALTER TABLE movie
    ADD COLUMN deleted_at BIGINT NOT NULL DEFAULT 0 COMMENT '软删除时间(毫秒), 0=未删',
    ADD KEY idx_deleted_at (deleted_at);

ALTER TABLE movie_search
    ADD COLUMN deleted_at BIGINT NOT NULL DEFAULT 0 COMMENT '软删除时间(毫秒), 0=未删 (镜像自 movie)',
    ADD KEY idx_deleted_at (deleted_at);
