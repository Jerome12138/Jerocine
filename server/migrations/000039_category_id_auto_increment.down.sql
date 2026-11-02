-- 回滚 000039: 去掉 category.id 的 AUTO_INCREMENT(回到 000001 的列定义)。
-- 注意: 回滚前需保证 max(id) 行没有被 AUTO_INCREMENT 语义依赖; 该操作只改列元数据, 不动数据。
ALTER TABLE category
    MODIFY COLUMN id BIGINT NOT NULL;
