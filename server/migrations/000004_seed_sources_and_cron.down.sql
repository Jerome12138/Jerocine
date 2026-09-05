-- 000004 down: 删除种子数据
DELETE FROM cron_task WHERE remark = '默认自动更新(初始停用)';
DELETE FROM site_config WHERE id = 1;
DELETE FROM collect_source WHERE id IN ('src_lz', 'src_sn', 'src_bf', 'src_ff', 'src_kk');
