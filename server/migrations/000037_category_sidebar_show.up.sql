-- 000037 分类三开关拆分: 顶栏(show) / 侧边栏(sidebar_show) / 影片可见(film_visible)。
-- 语义变更: show 从"菜单显隐(一级+二级共用)"收窄为"顶栏导航显示(仅一级有意义)";
-- 侧边栏(桌面常驻侧栏 + 移动抽屉, 一级+二级)独立开关 sidebar_show。
-- 初始值 = show 现值(原"菜单隐藏"的分类侧边栏也保持隐藏, 不突变)。
-- film_visible 不变(false = 列表/搜索/推荐全剔除), 由服务层强制联动
-- (置 false 时节点+子孙的 show/sidebar_show 一并置 false)。
ALTER TABLE category
  ADD COLUMN sidebar_show TINYINT(1) NOT NULL DEFAULT 1 AFTER `show`;

UPDATE category SET sidebar_show = `show`;
