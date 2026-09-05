-- 000001 内容核心: movie / movie_search / movie_play_source / category
-- 设计要点(对齐 doc/重构方案-全栈终态蓝图.md §3.1):
--   * 业务表禁软删除(硬删 + state 业务状态), 主键 mid/id 全局唯一不嵌 cid
--   * created_at/updated_at 用 BIGINT 毫秒
--   * db_score DECIMAL(3,1) 存, 出参转 float64
--   * movie_search 为物化卡片/检索宽表(CQRS 读模型), cover 进表 → 列表零回填
--   * FULLTEXT ... WITH PARSER ngram 替代 LIKE '%kw%' 前导通配全表扫

-- 1) movie 影片详情主表 (去 cid 维度, mid 全局唯一一跳直查)
CREATE TABLE IF NOT EXISTS movie (
  mid           BIGINT       NOT NULL PRIMARY KEY,
  cid           BIGINT       NOT NULL DEFAULT 0,
  pid           BIGINT       NOT NULL DEFAULT 0,
  name          VARCHAR(255) NOT NULL,
  sub_title     VARCHAR(255) NOT NULL DEFAULT '',
  c_name        VARCHAR(64)  NOT NULL DEFAULT '',
  en_name       VARCHAR(255) NOT NULL DEFAULT '',
  initial       VARCHAR(8)   NOT NULL DEFAULT '',
  class_tag     VARCHAR(255) NOT NULL DEFAULT '',
  area          VARCHAR(64)  NOT NULL DEFAULT '',
  language      VARCHAR(64)  NOT NULL DEFAULT '',
  year          INT          NOT NULL DEFAULT 0,
  actor         TEXT,
  director      TEXT,
  writer        TEXT,
  content       MEDIUMTEXT,
  db_id         BIGINT       NOT NULL DEFAULT 0,
  db_score      DECIMAL(3,1) NOT NULL DEFAULT 0,
  hits          BIGINT       NOT NULL DEFAULT 0,
  state         VARCHAR(32)  NOT NULL DEFAULT '',
  remarks       VARCHAR(64)  NOT NULL DEFAULT '',
  cover         VARCHAR(512) NOT NULL DEFAULT '',
  play_from     JSON,
  down_from     VARCHAR(255) NOT NULL DEFAULT '',
  release_stamp BIGINT       NOT NULL DEFAULT 0,
  update_stamp  BIGINT       NOT NULL DEFAULT 0,
  created_at    BIGINT       NOT NULL,
  updated_at    BIGINT       NOT NULL,
  KEY idx_pid_release(pid, release_stamp),
  KEY idx_pid_hits(pid, hits),
  KEY idx_pid_update(pid, update_stamp),
  KEY idx_cid(cid),
  FULLTEXT KEY ft_name(name, sub_title) WITH PARSER ngram
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2) movie_search 物化卡片/检索宽表 (mid PK 重采无冲突, cover 进表零回填)
CREATE TABLE IF NOT EXISTS movie_search (
  mid           BIGINT       NOT NULL PRIMARY KEY,
  cid           BIGINT       NOT NULL DEFAULT 0,
  pid           BIGINT       NOT NULL DEFAULT 0,
  name          VARCHAR(255) NOT NULL,
  sub_title     VARCHAR(255) NOT NULL DEFAULT '',
  c_name        VARCHAR(64)  NOT NULL DEFAULT '',
  class_tag     VARCHAR(255) NOT NULL DEFAULT '',
  area          VARCHAR(64)  NOT NULL DEFAULT '',
  language      VARCHAR(64)  NOT NULL DEFAULT '',
  year          INT          NOT NULL DEFAULT 0,
  initial       VARCHAR(8)   NOT NULL DEFAULT '',
  state         VARCHAR(32)  NOT NULL DEFAULT '',
  remarks       VARCHAR(64)  NOT NULL DEFAULT '',
  db_score      DECIMAL(3,1) NOT NULL DEFAULT 0,
  hits          BIGINT       NOT NULL DEFAULT 0,
  cover         VARCHAR(512) NOT NULL DEFAULT '',
  release_stamp BIGINT       NOT NULL DEFAULT 0,
  update_stamp  BIGINT       NOT NULL DEFAULT 0,
  created_at    BIGINT       NOT NULL,
  updated_at    BIGINT       NOT NULL,
  KEY idx_pid_release(pid, release_stamp),
  KEY idx_pid_hits(pid, hits),
  KEY idx_pid_update(pid, update_stamp),
  KEY idx_pid_year(pid, year),
  KEY idx_filter(pid, cid, area, language, year),
  KEY idx_cid(cid),
  FULLTEXT KEY ft_search(name, sub_title) WITH PARSER ngram
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3) movie_play_source 多源播放 (episodes_json 整源一行)
CREATE TABLE IF NOT EXISTS movie_play_source (
  id            BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
  mid           BIGINT      NULL,
  site_id       VARCHAR(64) NOT NULL,
  match_key     VARCHAR(64) NOT NULL,
  play_from     VARCHAR(64) NOT NULL,
  episodes_json JSON        NOT NULL,
  created_at    BIGINT      NOT NULL,
  updated_at    BIGINT      NOT NULL,
  UNIQUE KEY uk_site_match_from(site_id, match_key, play_from),
  KEY idx_mid(mid),
  KEY idx_match(match_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 4) category 分类父子表 (行级 CRUD)
CREATE TABLE IF NOT EXISTS category (
  id         BIGINT      NOT NULL PRIMARY KEY,
  pid        BIGINT      NOT NULL DEFAULT 0,
  name       VARCHAR(64) NOT NULL,
  `show`     TINYINT     NOT NULL DEFAULT 1,
  sort       INT         NOT NULL DEFAULT 0,
  created_at BIGINT      NOT NULL,
  updated_at BIGINT      NOT NULL,
  KEY idx_pid(pid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
