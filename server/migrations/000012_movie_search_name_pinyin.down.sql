-- 回滚 000012: 移除 name_pinyin 列与索引。
ALTER TABLE movie_search
  DROP INDEX idx_name_pinyin,
  DROP COLUMN name_pinyin;
