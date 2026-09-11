-- 000017: 影片横图(backdrop) —— TMDB 按片名检索 + 下载到本地 blob 后回填。
-- movie = 详情真值, movie_search = 卡片/轮播兜底读模型; 两表同列, 由后台 worker 同步回填。
-- 采集 upsert 把 backdrop 排除在更新列之外(见 movieUpsertExclude / searchUpsertExclude),
-- 保证源站重推不会把已回填的横图抹掉。

ALTER TABLE movie
  ADD COLUMN backdrop VARCHAR(255) NOT NULL DEFAULT '' AFTER cover;

ALTER TABLE movie_search
  ADD COLUMN backdrop VARCHAR(255) NOT NULL DEFAULT '' AFTER cover;
