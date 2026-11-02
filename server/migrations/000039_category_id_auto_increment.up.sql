-- 000039 分类表主键补 AUTO_INCREMENT —— 修复「后台手工新增分类」必失败。
-- 背景: category.id 自 000001 起是 BIGINT NOT NULL PRIMARY KEY(无自增、无默认值);
-- 管理端「新增分类」以 id=0 走 Upsert(gorm OnConflict(id)+UpdateAll), gorm 对零值
-- 整型主键按"由 DB 生成"处理, 生成的 INSERT 整体省略 id 列:
--   INSERT INTO `category` (`pid`,`name`,...) VALUES (...) ON DUPLICATE KEY UPDATE ...
-- 在 MySQL 严格模式(默认)下, NOT NULL 无默认值的 id 缺参直接报 1366 → dto.Fail 500;
-- 非严格模式则落 id=0 幽灵行, 第二次新增覆盖同一行。
-- 存量行的 id 都来自采集源(type_id)或手工指定, ALTER 后 AUTO_INCREMENT 计数会自动
-- 对齐 max(id)+1, 不影响既有数据; 编辑已有分类(id 非零)路径不变(INSERT 显式带 id, ODKU 命中)。
ALTER TABLE category
    MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT;
