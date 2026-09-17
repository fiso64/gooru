ALTER TABLE background_operations ADD COLUMN result_outcome TEXT;
ALTER TABLE background_operations ADD COLUMN result_affected_count INTEGER;
ALTER TABLE background_operations ADD COLUMN result_failed_count INTEGER;
