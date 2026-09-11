-- 000017 down: 移除横图列。

ALTER TABLE movie
  DROP COLUMN backdrop;

ALTER TABLE movie_search
  DROP COLUMN backdrop;
