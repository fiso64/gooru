-- This migration defines the "v2" schema changes.

-- Add the files_count column with a default value, making it non-nullable.
ALTER TABLE tags ADD COLUMN files_count INTEGER NOT NULL DEFAULT 0;

-- Create an index on the new column to make sorting by count fast.
CREATE INDEX idx_tags_files_count ON tags(files_count);

-- Back-fill the counts for all tags that already exist in the database.
-- This is a critical one-time operation for existing databases.
UPDATE tags
SET files_count = (
    SELECT COUNT(content_hash)
    FROM content_tags
    WHERE tag_id = tags.id
);

-- Drop the old trigger that used an inefficient subquery to check for orphans.
DROP TRIGGER cleanup_orphan_tags_on_delete;

-- Create new, highly efficient triggers to maintain the files_count.
CREATE TRIGGER increment_tag_count_on_insert
    AFTER INSERT ON content_tags
    BEGIN
        UPDATE tags SET files_count = files_count + 1 WHERE id = NEW.tag_id;
    END;

CREATE TRIGGER decrement_tag_count_on_delete
    AFTER DELETE ON content_tags
    BEGIN
        UPDATE tags SET files_count = files_count - 1 WHERE id = OLD.tag_id;
    END;

-- This trigger now efficiently cleans up tags when their count hits zero,
-- replacing the old `cleanup_orphan_tags_on_delete` trigger.
CREATE TRIGGER cleanup_zero_count_tags_after_update
    AFTER UPDATE OF files_count ON tags
    BEGIN
        DELETE FROM tags WHERE id = NEW.id AND files_count <= 0;
    END;