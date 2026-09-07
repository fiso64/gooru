-- Maintain per-tag effective media-kind counts so a simple exact-tag browse can
-- answer kind facets without scanning every matching library location.
-- Counts are per tracked location, matching KindFacetsByLocationQuery semantics.
CREATE TABLE tag_kind_counts (
    tag_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    files_count INTEGER NOT NULL,
    PRIMARY KEY (tag_id, kind),
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

INSERT INTO tag_kind_counts (tag_id, kind, files_count)
SELECT ct.tag_id,
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
FROM content_tags ct
JOIN locations l ON l.content_hash = ct.content_hash
LEFT JOIN media_metadata mm ON mm.location_id = l.id
GROUP BY ct.tag_id, kind;

CREATE TRIGGER tag_kind_counts_content_tag_insert
AFTER INSERT ON content_tags
BEGIN
    INSERT INTO tag_kind_counts (tag_id, kind, files_count)
    SELECT NEW.tag_id,
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
    ON CONFLICT(tag_id, kind) DO UPDATE SET files_count = files_count + excluded.files_count;
END;

CREATE TRIGGER tag_kind_counts_content_tag_delete
AFTER DELETE ON content_tags
BEGIN
    UPDATE tag_kind_counts
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
          END = tag_kind_counts.kind
    )
    WHERE tag_id = OLD.tag_id;
    DELETE FROM tag_kind_counts WHERE tag_id = OLD.tag_id AND files_count <= 0;
END;

CREATE TRIGGER tag_kind_counts_location_insert
AFTER INSERT ON locations
BEGIN
    INSERT INTO tag_kind_counts (tag_id, kind, files_count)
    SELECT ct.tag_id,
           CASE
               WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
               WHEN lower(NEW.extension) = '.gif' THEN 'gif'
               WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
               WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
               ELSE 'other'
           END,
           1
    FROM content_tags ct
    WHERE ct.content_hash = NEW.content_hash
    ON CONFLICT(tag_id, kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER tag_kind_counts_location_delete
BEFORE DELETE ON locations
BEGIN
    UPDATE tag_kind_counts
    SET files_count = files_count - 1
    WHERE tag_id IN (SELECT tag_id FROM content_tags WHERE content_hash = OLD.content_hash)
      AND kind = CASE
          WHEN lower(OLD.extension) = '.cbz' THEN 'comic'
          ELSE coalesce(
              (SELECT media_kind FROM media_metadata WHERE location_id = OLD.id),
              CASE
                  WHEN lower(OLD.extension) = '.gif' THEN 'gif'
                  WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                  WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                  ELSE 'other'
              END
          )
      END;
    DELETE FROM tag_kind_counts
    WHERE tag_id IN (SELECT tag_id FROM content_tags WHERE content_hash = OLD.content_hash)
      AND files_count <= 0;
END;

CREATE TRIGGER tag_kind_counts_location_extension_update
AFTER UPDATE OF extension ON locations
WHEN OLD.extension IS NOT NEW.extension
BEGIN
    UPDATE tag_kind_counts
    SET files_count = files_count - 1
    WHERE tag_id IN (SELECT tag_id FROM content_tags WHERE content_hash = NEW.content_hash)
      AND kind = CASE
          WHEN lower(OLD.extension) = '.cbz' THEN 'comic'
          ELSE coalesce(
              (SELECT media_kind FROM media_metadata WHERE location_id = NEW.id),
              CASE
                  WHEN lower(OLD.extension) = '.gif' THEN 'gif'
                  WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                  WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                  ELSE 'other'
              END
          )
      END;

    INSERT INTO tag_kind_counts (tag_id, kind, files_count)
    SELECT ct.tag_id,
           CASE
               WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
               ELSE coalesce(
                   (SELECT media_kind FROM media_metadata WHERE location_id = NEW.id),
                   CASE
                       WHEN lower(NEW.extension) = '.gif' THEN 'gif'
                       WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                       WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                       ELSE 'other'
                   END
               )
           END,
           1
    FROM content_tags ct
    WHERE ct.content_hash = NEW.content_hash
    ON CONFLICT(tag_id, kind) DO UPDATE SET files_count = files_count + 1;

    DELETE FROM tag_kind_counts
    WHERE tag_id IN (SELECT tag_id FROM content_tags WHERE content_hash = NEW.content_hash)
      AND files_count <= 0;
END;

CREATE TRIGGER tag_kind_counts_metadata_insert
AFTER INSERT ON media_metadata
BEGIN
    UPDATE tag_kind_counts
    SET files_count = files_count - 1
    WHERE tag_id IN (
        SELECT ct.tag_id FROM content_tags ct
        JOIN locations l ON l.content_hash = ct.content_hash
        WHERE l.id = NEW.location_id
    )
      AND kind = (
          SELECT CASE
              WHEN lower(extension) = '.cbz' THEN 'comic'
              WHEN lower(extension) = '.gif' THEN 'gif'
              WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
              WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
              ELSE 'other'
          END
          FROM locations WHERE id = NEW.location_id
      );

    INSERT INTO tag_kind_counts (tag_id, kind, files_count)
    SELECT ct.tag_id,
           CASE WHEN lower(l.extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END,
           1
    FROM locations l
    JOIN content_tags ct ON ct.content_hash = l.content_hash
    WHERE l.id = NEW.location_id
    ON CONFLICT(tag_id, kind) DO UPDATE SET files_count = files_count + 1;

    DELETE FROM tag_kind_counts WHERE files_count <= 0;
END;

CREATE TRIGGER tag_kind_counts_metadata_update
AFTER UPDATE OF media_kind ON media_metadata
WHEN OLD.media_kind IS NOT NEW.media_kind
BEGIN
    UPDATE tag_kind_counts
    SET files_count = files_count - 1
    WHERE tag_id IN (
        SELECT ct.tag_id FROM content_tags ct
        JOIN locations l ON l.content_hash = ct.content_hash
        WHERE l.id = NEW.location_id
    )
      AND kind = (
          SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END
          FROM locations WHERE id = NEW.location_id
      );

    INSERT INTO tag_kind_counts (tag_id, kind, files_count)
    SELECT ct.tag_id,
           CASE WHEN lower(l.extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END,
           1
    FROM locations l
    JOIN content_tags ct ON ct.content_hash = l.content_hash
    WHERE l.id = NEW.location_id
    ON CONFLICT(tag_id, kind) DO UPDATE SET files_count = files_count + 1;

    DELETE FROM tag_kind_counts WHERE files_count <= 0;
END;

CREATE TRIGGER tag_kind_counts_metadata_delete
AFTER DELETE ON media_metadata
WHEN EXISTS (SELECT 1 FROM locations WHERE id = OLD.location_id)
BEGIN
    UPDATE tag_kind_counts
    SET files_count = files_count - 1
    WHERE tag_id IN (
        SELECT ct.tag_id FROM content_tags ct
        JOIN locations l ON l.content_hash = ct.content_hash
        WHERE l.id = OLD.location_id
    )
      AND kind = (
          SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END
          FROM locations WHERE id = OLD.location_id
      );

    INSERT INTO tag_kind_counts (tag_id, kind, files_count)
    SELECT ct.tag_id,
           CASE
               WHEN lower(l.extension) = '.cbz' THEN 'comic'
               WHEN lower(l.extension) = '.gif' THEN 'gif'
               WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
               WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
               ELSE 'other'
           END,
           1
    FROM locations l
    JOIN content_tags ct ON ct.content_hash = l.content_hash
    WHERE l.id = OLD.location_id
    ON CONFLICT(tag_id, kind) DO UPDATE SET files_count = files_count + 1;

    DELETE FROM tag_kind_counts WHERE files_count <= 0;
END;
