DROP TRIGGER IF EXISTS delete_tag_associations_before_tag_delete;
DROP TRIGGER IF EXISTS decrement_tag_counts_on_delete;
DROP TRIGGER IF EXISTS increment_tag_counts_on_insert;
DROP TABLE IF EXISTS tag_key_counts;

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
