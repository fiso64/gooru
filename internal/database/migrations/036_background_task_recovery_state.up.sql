ALTER TABLE background_tasks
ADD COLUMN checkpoint_json TEXT NOT NULL DEFAULT '';

ALTER TABLE background_tasks
ADD COLUMN result_json TEXT NOT NULL DEFAULT '';
