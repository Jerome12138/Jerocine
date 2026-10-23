-- 000030 分类「菜单显隐」与「影片可见」双开关拆分。
-- 此前 category.show 一个字段同时控制两件事: ①导航菜单是否显示该分类;
-- ②该分类影片从公开列表/搜索中剔除(search_repo hiddenCats)。
-- 拆分后: show = 纯菜单显隐(影片仍可搜索/在列表看到);
--         film_visible = 影片根本可见性(关闭即全出口剔除, 管理口径除外)。
-- 存量数据回填: film_visible = show, 保持拆分前"隐藏菜单=影片也藏"的行为不变。
ALTER TABLE category
    ADD COLUMN film_visible TINYINT(1) NOT NULL DEFAULT 1 COMMENT '影片可见: 0=该分类影片从公开列表/搜索全部剔除(菜单显隐用 show 列)' AFTER `show`;
UPDATE category SET film_visible = `show`;
