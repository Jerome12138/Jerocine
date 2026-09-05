-- 000012 movie_search 增加片名拼音首字母串(name_pinyin), 支持首字母搜索(如 凡人修仙传 → FRXXZ)。
-- 旧行默认空串; film_api 启动时一次性回填(repomysql.BackfillNamePinyin, 仅空值行),
--   之后采集/手动加片经 ProjectMovieToSearch 自动写入。
-- 影子表 movie_search_next 由 `CREATE TABLE ... LIKE movie_search` 建, 列与索引自动继承。
-- 单条 ALTER(双子句)避免 multiStatements 依赖。
ALTER TABLE movie_search
  ADD COLUMN name_pinyin VARCHAR(64) NOT NULL DEFAULT '' AFTER initial,
  ADD INDEX idx_name_pinyin (name_pinyin);
