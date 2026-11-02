-- 回滚 000037: 移除侧边栏独立开关列(顶栏/侧边栏重新合并回 show)。
ALTER TABLE category DROP COLUMN sidebar_show;
