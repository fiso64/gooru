UPDATE meta SET value = '1' WHERE key = 'db_version';

-- This reverts existing cache entries to use commas.
UPDATE locations SET tags_cache = REPLACE(tags_cache, ' ', ',');

DROP TRIGGER populate_tags_cache_on_location_insert;
DROP TRIGGER update_tags_cache_on_insert;
DROP TRIGGER update_tags_cache_on_delete;

-- Re-create triggers with default comma delimiter.
CREATE TRIGGER populate_tags_cache_on_location_insert
    AFTER INSERT ON locations
    BEGIN
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = NEW.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE id = NEW.id;
    END;

CREATE TRIGGER update_tags_cache_on_insert
    AFTER INSERT ON content_tags
    BEGIN
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = NEW.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE content_hash = NEW.content_hash;
    END;

CREATE TRIGGER update_tags_cache_on_delete
    AFTER DELETE ON content_tags
    BEGIN
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = OLD.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE content_hash = OLD.content_hash;
    END;