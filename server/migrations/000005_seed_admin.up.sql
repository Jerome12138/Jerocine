-- 000005 seed: 默认管理员 admin / change_me_admin (bcrypt, cost 10)。
-- 任何新库初始化自动创建; 幂等(已存在则跳过)。
-- ⚠ 默认凭证仅用于首次登录, 公网部署后必须立即修改密码。
INSERT INTO users (created_at, updated_at, user_name, password, role)
SELECT NOW(3), NOW(3), 'admin', '$2a$10$c3H7MtgR8LP16pqHs244V.hrFnMOwJ1aeB5EOZUH7.5NEvwc39YYG', 1
WHERE NOT EXISTS (SELECT 1 FROM users WHERE user_name = 'admin');
