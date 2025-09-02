-- The reverse of the 'Up' migration, for development/testing purposes.

DROP TRIGGER cleanup_zero_count_tags_after_update;
DROP TRIGGER decrement_tag_count_on_delete;
DROP TRIGGER increment_tag_count_on_insert;

-- Re-create the old, inefficient trigger.
CREATE TRIGGER cleanup_orphan_tags_on_delete
    AFTER DELETE ON content_tags
    BEGIN
        DELETE FROM tags
        WHERE id = OLD.tag_id
          AND NOT EXISTS (
            SELECT 1 FROM content_tags WHERE tag_id = OLD.tag_id
        );
    END;

DROP INDEX idx_tags_files_count;

-- SQLite does not support DROP COLUMN directly. A full table rebuild is needed.
-- This is complex and often omitted for "down" migrations in SQLite.
-- The column will just be ignored if this "down" migration is run.