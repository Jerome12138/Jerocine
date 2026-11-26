ALTER TABLE task_run
  DROP KEY idx_status_ignored_started,
  DROP COLUMN ignored;
