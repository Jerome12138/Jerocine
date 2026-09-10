ALTER TABLE movie_search
    DROP KEY idx_deleted_at,
    DROP COLUMN deleted_at;

ALTER TABLE movie
    DROP KEY idx_deleted_at,
    DROP COLUMN deleted_at;
