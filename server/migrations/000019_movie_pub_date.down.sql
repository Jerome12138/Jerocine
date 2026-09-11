-- 000019 down: 移除上映日期列。

ALTER TABLE movie
  DROP COLUMN pub_date;

ALTER TABLE movie_search
  DROP COLUMN pub_date;
