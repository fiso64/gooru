-- Tag rename is an infrequent administrative operation, so favor a bounded,
-- obviously-correct rebuild of the affected namespaces over more fragile
-- per-content delta bookkeeping. This also repairs the pre-existing key and
-- namespace count summaries when RenameTag updates tags.key/value in place.
CREATE TRIGGER rebuild_tag_key_summaries_on_tag_rename
AFTER UPDATE OF key, value ON tags
WHEN OLD.key IS NOT NEW.key OR OLD.value IS NOT NEW.value
BEGIN
    DELETE FROM tag_key_counts
    WHERE key = OLD.key COLLATE NOCASE OR key = NEW.key COLLATE NOCASE;

    INSERT INTO tag_key_counts (key, files_count)
    SELECT t.key, COUNT(DISTINCT ct.content_hash)
    FROM tags t
    JOIN content_tags ct ON ct.tag_id = t.id
    WHERE t.key = OLD.key COLLATE NOCASE OR t.key = NEW.key COLLATE NOCASE
    GROUP BY t.key;

    DELETE FROM tag_namespace_counts
    WHERE key = OLD.key COLLATE NOCASE OR key = NEW.key COLLATE NOCASE;

    INSERT INTO tag_namespace_counts (key, files_count)
    SELECT t.key, COUNT(DISTINCT ct.content_hash)
    FROM tags t
    JOIN content_tags ct ON ct.tag_id = t.id
    WHERE t.value != ''
      AND (t.key = OLD.key COLLATE NOCASE OR t.key = NEW.key COLLATE NOCASE)
    GROUP BY t.key;

    DELETE FROM tag_key_kind_counts
    WHERE key = OLD.key COLLATE NOCASE OR key = NEW.key COLLATE NOCASE;

    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT tagged.key,
           CASE
               WHEN lower(l.extension) = '.cbz' THEN 'comic'
               ELSE coalesce(mm.media_kind, CASE
                   WHEN lower(l.extension) = '.gif' THEN 'gif'
                   WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                   WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                   ELSE 'other'
               END)
           END AS kind,
           COUNT(*)
    FROM (
        SELECT DISTINCT ct.content_hash, t.key
        FROM content_tags ct
        JOIN tags t ON t.id = ct.tag_id
        WHERE t.key = OLD.key COLLATE NOCASE OR t.key = NEW.key COLLATE NOCASE
    ) tagged
    JOIN locations l ON l.content_hash = tagged.content_hash
    LEFT JOIN media_metadata mm ON mm.location_id = l.id
    GROUP BY tagged.key, kind;
END;
