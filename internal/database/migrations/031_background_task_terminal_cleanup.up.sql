ALTER TABLE background_tasks
ADD COLUMN terminal_cleanup_json TEXT NOT NULL DEFAULT '';
