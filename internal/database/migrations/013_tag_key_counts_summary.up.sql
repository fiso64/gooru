-- The tag-count API synthesizes one aggregate row per key. Maintain those
-- distinct-per-content key counts incrementally rather than regrouping every
-- content_tags row on each request.
CREATE TABLE tag_key_counts (
    key TEXT PRIMARY KEY COLLATE NOCASE,
    files_count INTEGER NOT NULL CHECK (files_count >= 0)
);

INSERT INTO tag_key_counts (key, files_count)
SELECT t.key, COUNT(DISTINCT ct.content_hash)
FROM tags t
JOIN content_tags ct ON ct.tag_id = t.id
GROUP BY t.key;

-- Replace the v2 per-tag triggers so key-level and value-level counts are
-- updated in a deterministic order. The existing zero-count cleanup trigger
-- remains responsible for removing orphaned tag rows.
DROP TRIGGER increment_tag_count_on_insert;
DROP TRIGGER decrement_tag_count_on_delete;

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

-- Ensure even an explicit tag-row deletion routes association removal through
-- the count-maintenance trigger while OLD.id/key are still queryable. Normal
-- zero-count cleanup reaches this trigger with no associations and is a no-op.
CREATE TRIGGER delete_tag_associations_before_tag_delete
BEFORE DELETE ON tags
BEGIN
    DELETE FROM content_tags WHERE tag_id = OLD.id;
END;
