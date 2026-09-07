DROP TRIGGER increment_tag_counts_on_insert;
DROP TRIGGER decrement_tag_counts_on_delete;
DROP TABLE tag_namespace_counts;

CREATE TRIGGER increment_tag_counts_on_insert
AFTER INSERT ON content_tags
BEGIN
    UPDATE tags SET files_count = files_count + 1 WHERE id = NEW.tag_id;

    INSERT INTO tag_key_counts (key, files_count)
    SELECT t.key, 1
    FROM tags t
    WHERE t.id = NEW.tag_id
      AND NOT EXISTS (
          SELECT 1
          FROM content_tags ct
          JOIN tags other ON other.id = ct.tag_id
          WHERE ct.content_hash = NEW.content_hash
            AND other.key = t.key
            AND ct.tag_id != NEW.tag_id
      )
    ON CONFLICT(key) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER decrement_tag_counts_on_delete
AFTER DELETE ON content_tags
BEGIN
    UPDATE tag_key_counts
    SET files_count = files_count - 1
    WHERE key = (SELECT key FROM tags WHERE id = OLD.tag_id)
      AND NOT EXISTS (
          SELECT 1
          FROM content_tags ct
          JOIN tags other ON other.id = ct.tag_id
          WHERE ct.content_hash = OLD.content_hash
            AND other.key = (SELECT key FROM tags WHERE id = OLD.tag_id)
      );

    UPDATE tags SET files_count = files_count - 1 WHERE id = OLD.tag_id;
END;
