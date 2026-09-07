-- Maintain per-tag-key effective media-kind counts for key-wide queries such as
-- `hidden`. A content contributes each tracked location once per key, regardless
-- of how many values it has in that namespace.
CREATE TABLE tag_key_kind_counts (
    key TEXT NOT NULL,
    kind TEXT NOT NULL,
    files_count INTEGER NOT NULL,
    PRIMARY KEY (key, kind)
);

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
) tagged
JOIN locations l ON l.content_hash = tagged.content_hash
LEFT JOIN media_metadata mm ON mm.location_id = l.id
GROUP BY tagged.key, kind;

CREATE TRIGGER tag_key_kind_counts_content_tag_insert
AFTER INSERT ON content_tags
WHEN (
    SELECT COUNT(*) FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_hash = NEW.content_hash
      AND t.key = (SELECT key FROM tags WHERE id = NEW.tag_id)
) = 1
BEGIN
    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT (SELECT key FROM tags WHERE id = NEW.tag_id),
           CASE
               WHEN lower(l.extension) = '.cbz' THEN 'comic'
               ELSE coalesce(mm.media_kind, CASE
                   WHEN lower(l.extension) = '.gif' THEN 'gif'
                   WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                   WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                   ELSE 'other'
               END)
           END,
           COUNT(*)
    FROM locations l
    LEFT JOIN media_metadata mm ON mm.location_id = l.id
    WHERE l.content_hash = NEW.content_hash
    GROUP BY 2
    ON CONFLICT(key, kind) DO UPDATE SET files_count = files_count + excluded.files_count;
END;

CREATE TRIGGER tag_key_kind_counts_content_tag_delete
AFTER DELETE ON content_tags
WHEN NOT EXISTS (
    SELECT 1 FROM content_tags ct
    JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_hash = OLD.content_hash
      AND t.key = (SELECT key FROM tags WHERE id = OLD.tag_id)
)
BEGIN
    UPDATE tag_key_kind_counts
    SET files_count = files_count - (
        SELECT COUNT(*)
        FROM locations l
        LEFT JOIN media_metadata mm ON mm.location_id = l.id
        WHERE l.content_hash = OLD.content_hash
          AND CASE
              WHEN lower(l.extension) = '.cbz' THEN 'comic'
              ELSE coalesce(mm.media_kind, CASE
                  WHEN lower(l.extension) = '.gif' THEN 'gif'
                  WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                  WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                  ELSE 'other'
              END)
          END = tag_key_kind_counts.kind
    )
    WHERE key = (SELECT key FROM tags WHERE id = OLD.tag_id);
    DELETE FROM tag_key_kind_counts
    WHERE key = (SELECT key FROM tags WHERE id = OLD.tag_id) AND files_count <= 0;
END;

CREATE TRIGGER tag_key_kind_counts_location_insert
AFTER INSERT ON locations
BEGIN
    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT DISTINCT t.key,
           CASE
               WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
               WHEN lower(NEW.extension) = '.gif' THEN 'gif'
               WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
               WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
               ELSE 'other'
           END,
           1
    FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_hash = NEW.content_hash
    ON CONFLICT(key, kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER tag_key_kind_counts_location_delete
BEFORE DELETE ON locations
BEGIN
    UPDATE tag_key_kind_counts
    SET files_count = files_count - 1
    WHERE key IN (
        SELECT DISTINCT t.key FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
        WHERE ct.content_hash = OLD.content_hash
    )
      AND kind = CASE
          WHEN lower(OLD.extension) = '.cbz' THEN 'comic'
          ELSE coalesce((SELECT media_kind FROM media_metadata WHERE location_id = OLD.id), CASE
              WHEN lower(OLD.extension) = '.gif' THEN 'gif'
              WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
              WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
              ELSE 'other'
          END)
      END;
    DELETE FROM tag_key_kind_counts WHERE files_count <= 0;
END;

CREATE TRIGGER tag_key_kind_counts_location_extension_update
AFTER UPDATE OF extension ON locations
WHEN OLD.extension IS NOT NEW.extension
BEGIN
    UPDATE tag_key_kind_counts
    SET files_count = files_count - 1
    WHERE key IN (
        SELECT DISTINCT t.key FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
        WHERE ct.content_hash = NEW.content_hash
    )
      AND kind = CASE
          WHEN lower(OLD.extension) = '.cbz' THEN 'comic'
          ELSE coalesce((SELECT media_kind FROM media_metadata WHERE location_id = NEW.id), CASE
              WHEN lower(OLD.extension) = '.gif' THEN 'gif'
              WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
              WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
              ELSE 'other'
          END)
      END;
    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT DISTINCT t.key,
           CASE
               WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
               ELSE coalesce((SELECT media_kind FROM media_metadata WHERE location_id = NEW.id), CASE
                   WHEN lower(NEW.extension) = '.gif' THEN 'gif'
                   WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                   WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                   ELSE 'other'
               END)
           END,
           1
    FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
    WHERE ct.content_hash = NEW.content_hash
    ON CONFLICT(key, kind) DO UPDATE SET files_count = files_count + 1;
    DELETE FROM tag_key_kind_counts WHERE files_count <= 0;
END;

CREATE TRIGGER tag_key_kind_counts_metadata_insert
AFTER INSERT ON media_metadata
BEGIN
    UPDATE tag_key_kind_counts
    SET files_count = files_count - 1
    WHERE key IN (
        SELECT DISTINCT t.key FROM locations l
        JOIN content_tags ct ON ct.content_hash = l.content_hash
        JOIN tags t ON t.id = ct.tag_id
        WHERE l.id = NEW.location_id
    )
      AND kind = (SELECT CASE
          WHEN lower(extension) = '.cbz' THEN 'comic'
          WHEN lower(extension) = '.gif' THEN 'gif'
          WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
          WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
          ELSE 'other' END FROM locations WHERE id = NEW.location_id);
    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT DISTINCT t.key, CASE WHEN lower(l.extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END, 1
    FROM locations l
    JOIN content_tags ct ON ct.content_hash = l.content_hash
    JOIN tags t ON t.id = ct.tag_id
    WHERE l.id = NEW.location_id
    ON CONFLICT(key, kind) DO UPDATE SET files_count = files_count + 1;
    DELETE FROM tag_key_kind_counts WHERE files_count <= 0;
END;

CREATE TRIGGER tag_key_kind_counts_metadata_update
AFTER UPDATE OF media_kind ON media_metadata
WHEN OLD.media_kind IS NOT NEW.media_kind
BEGIN
    UPDATE tag_key_kind_counts SET files_count = files_count - 1
    WHERE key IN (
        SELECT DISTINCT t.key FROM locations l JOIN content_tags ct ON ct.content_hash = l.content_hash
        JOIN tags t ON t.id = ct.tag_id WHERE l.id = NEW.location_id
    ) AND kind = (SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END FROM locations WHERE id = NEW.location_id);
    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT DISTINCT t.key, CASE WHEN lower(l.extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END, 1
    FROM locations l JOIN content_tags ct ON ct.content_hash = l.content_hash JOIN tags t ON t.id = ct.tag_id
    WHERE l.id = NEW.location_id
    ON CONFLICT(key, kind) DO UPDATE SET files_count = files_count + 1;
    DELETE FROM tag_key_kind_counts WHERE files_count <= 0;
END;

CREATE TRIGGER tag_key_kind_counts_metadata_delete
AFTER DELETE ON media_metadata
WHEN EXISTS (SELECT 1 FROM locations WHERE id = OLD.location_id)
BEGIN
    UPDATE tag_key_kind_counts SET files_count = files_count - 1
    WHERE key IN (
        SELECT DISTINCT t.key FROM locations l JOIN content_tags ct ON ct.content_hash = l.content_hash
        JOIN tags t ON t.id = ct.tag_id WHERE l.id = OLD.location_id
    ) AND kind = (SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END FROM locations WHERE id = OLD.location_id);
    INSERT INTO tag_key_kind_counts (key, kind, files_count)
    SELECT DISTINCT t.key,
           CASE
               WHEN lower(l.extension) = '.cbz' THEN 'comic'
               WHEN lower(l.extension) = '.gif' THEN 'gif'
               WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
               WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
               ELSE 'other'
           END,
           1
    FROM locations l JOIN content_tags ct ON ct.content_hash = l.content_hash JOIN tags t ON t.id = ct.tag_id
    WHERE l.id = OLD.location_id
    ON CONFLICT(key, kind) DO UPDATE SET files_count = files_count + 1;
    DELETE FROM tag_key_kind_counts WHERE files_count <= 0;
END;
