-- 000011 采集源增加"仅端侧可达"标记(client_only)。
-- 背景: bf 等片源被地域封, 服务器(腾讯云)永远访问不到 → 服务端测速/健康检查对它们恒失败,
--   还会误判连续失败而自动停采。client_only=1 的源: 服务端不测速、不计失败、不自动停采;
--   前端(端侧)另做可达性测速。默认 0(服务端可达, 行为不变)。
ALTER TABLE collect_source ADD COLUMN client_only TINYINT(1) NOT NULL DEFAULT 0;
