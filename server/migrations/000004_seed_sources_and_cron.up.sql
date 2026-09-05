-- 000004 seed: 预置采集源 + 站点配置单行兜底 + 默认 cron(初始停用)
-- 采集源沿用旧项目 SpiderInit.FilmSourceInit 的预置列表(苹果CMS /provide/vod 接口)。
-- 变更: 旧项目 5 站全为 Slave(只采播放源, 无主站则 movie 表无数据)。
--   新架构需要一个 Master 填充 movie/movie_search 详情, 故将最完整的 lziapi 设为 grade=0(主站),
--   其余 4 站为 grade=1(附属, 仅补充多源播放)。state=0 启用 / 1 停用; result_model=0 json; collect_type=0 视频。

INSERT IGNORE INTO collect_source
  (id, name, uri, result_model, grade, sync_pictures, collect_type, grade_score, interval_ms, state, created_at, updated_at)
VALUES
  ('src_lz', 'HD(lz)', 'https://cj.lziapi.com/api.php/provide/vod/',                 0, 0, 0, 0, 100,    0, 0, UNIX_TIMESTAMP()*1000, UNIX_TIMESTAMP()*1000),
  ('src_sn', 'HD(sn)', 'https://suoniapi.com/api.php/provide/vod/from/snm3u8/',      0, 1, 0, 0,  90, 2000, 0, UNIX_TIMESTAMP()*1000, UNIX_TIMESTAMP()*1000),
  ('src_bf', 'HD(bf)', 'https://bfzyapi.com/api.php/provide/vod/',                   0, 1, 0, 0,  80, 2500, 0, UNIX_TIMESTAMP()*1000, UNIX_TIMESTAMP()*1000),
  ('src_ff', 'HD(ff)', 'http://cj.ffzyapi.com/api.php/provide/vod/',                 0, 1, 0, 0,  70,    0, 0, UNIX_TIMESTAMP()*1000, UNIX_TIMESTAMP()*1000),
  ('src_kk', 'HD(kk)', 'https://kuaikan-api.com/api.php/provide/vod/from/kuaikan/',  0, 1, 0, 0,  60,    0, 0, UNIX_TIMESTAMP()*1000, UNIX_TIMESTAMP()*1000);

INSERT IGNORE INTO site_config (id, site_name, domain, logo, keyword, description, state, hint, updated_at)
VALUES (1, 'Jerocine', '', '', '影视,在线观看', 'Jerocine 影视站', 0, '', UNIX_TIMESTAMP() * 1000);

-- 默认自动更新 cron(初始停用, model=0 自动全站, 每 20 分钟), 由后台启用
INSERT INTO cron_task (source_ids, spec, time, model, state, remark, entry_id, last_run_at, created_at, updated_at)
SELECT NULL, '0 */20 * * * ?', 3, 0, 1, '默认自动更新(初始停用)', 0, 0, UNIX_TIMESTAMP() * 1000, UNIX_TIMESTAMP() * 1000
WHERE NOT EXISTS (SELECT 1 FROM cron_task);
