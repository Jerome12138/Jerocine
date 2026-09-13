-- 000024 跳过设置总开关: 本剧是否启用跳过片头/片尾。
-- 关闭时保留 intro_sec/outro_sec 原值, 仅播放侧按 0 生效(不用把秒数改成 0 丢原值)。
ALTER TABLE user_skip_setting
  ADD COLUMN enabled TINYINT(1) NOT NULL DEFAULT 1;
