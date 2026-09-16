-- Restore the pre-v38 location projection on downgrade. Content-level rows
-- are fanned out to every currently tracked location for that content.
DROP TRIGGER IF EXISTS kind_counts_location_insert;
DROP TRIGGER IF EXISTS kind_counts_location_delete;
DROP TRIGGER IF EXISTS kind_counts_location_extension_update;
DROP TRIGGER IF EXISTS kind_counts_location_identity_update;
DROP TRIGGER IF EXISTS kind_counts_metadata_insert;
DROP TRIGGER IF EXISTS kind_counts_metadata_update;
DROP TRIGGER IF EXISTS kind_counts_metadata_delete;
DROP TRIGGER IF EXISTS tag_kind_counts_content_tag_insert;
DROP TRIGGER IF EXISTS tag_kind_counts_content_tag_delete;
DROP TRIGGER IF EXISTS tag_kind_counts_location_insert;
DROP TRIGGER IF EXISTS tag_kind_counts_location_delete;
DROP TRIGGER IF EXISTS tag_kind_counts_location_extension_update;
DROP TRIGGER IF EXISTS tag_kind_counts_location_identity_update;
DROP TRIGGER IF EXISTS tag_kind_counts_metadata_insert;
DROP TRIGGER IF EXISTS tag_kind_counts_metadata_update;
DROP TRIGGER IF EXISTS tag_kind_counts_metadata_delete;
DROP TRIGGER IF EXISTS tag_key_kind_counts_content_tag_insert;
DROP TRIGGER IF EXISTS tag_key_kind_counts_content_tag_delete;
DROP TRIGGER IF EXISTS tag_key_kind_counts_location_insert;
DROP TRIGGER IF EXISTS tag_key_kind_counts_location_delete;
DROP TRIGGER IF EXISTS tag_key_kind_counts_location_extension_update;
DROP TRIGGER IF EXISTS tag_key_kind_counts_location_identity_update;
DROP TRIGGER IF EXISTS tag_key_kind_counts_metadata_insert;
DROP TRIGGER IF EXISTS tag_key_kind_counts_metadata_update;
DROP TRIGGER IF EXISTS tag_key_kind_counts_metadata_delete;
DROP TRIGGER IF EXISTS rebuild_tag_key_summaries_on_tag_rename;
DROP INDEX IF EXISTS idx_media_metadata_kind_lower_content;
CREATE TABLE media_metadata_by_location (
    location_id INTEGER PRIMARY KEY,
    media_kind TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    image_width INTEGER,
    image_height INTEGER,
    video_width INTEGER,
    video_height INTEGER,
    duration_seconds REAL,
    frame_count INTEGER,
    page_count INTEGER,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (location_id) REFERENCES locations(id) ON DELETE CASCADE
);
INSERT INTO media_metadata_by_location (
    location_id, media_kind, mime_type, image_width, image_height,
    video_width, video_height, duration_seconds, frame_count, page_count, updated_at
)
SELECT l.id, mm.media_kind, mm.mime_type, mm.image_width, mm.image_height,
       mm.video_width, mm.video_height, mm.duration_seconds, mm.frame_count, mm.page_count, mm.updated_at
FROM locations l JOIN media_metadata mm ON mm.content_hash = l.content_hash;
DROP TABLE media_metadata;
ALTER TABLE media_metadata_by_location RENAME TO media_metadata;
CREATE INDEX idx_media_metadata_kind_lower_location
    ON media_metadata(lower(media_kind), location_id);

DELETE FROM tag_kind_counts;
INSERT INTO tag_kind_counts (tag_id, kind, files_count)
SELECT ct.tag_id,
       CASE WHEN lower(l.extension) = '.cbz' THEN 'comic'
            ELSE coalesce(mm.media_kind, CASE
        WHEN lower(l.extension) = '.gif' THEN 'gif'
        WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
        WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
        ELSE 'other'
    END) END,
       COUNT(*)
FROM content_tags ct JOIN locations l ON l.content_hash = ct.content_hash
LEFT JOIN media_metadata mm ON mm.location_id = l.id
GROUP BY ct.tag_id, 2;

DELETE FROM tag_key_kind_counts;
INSERT INTO tag_key_kind_counts (key, kind, files_count)
SELECT tagged.key,
       CASE WHEN lower(l.extension) = '.cbz' THEN 'comic'
            ELSE coalesce(mm.media_kind, CASE
        WHEN lower(l.extension) = '.gif' THEN 'gif'
        WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
        WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
        ELSE 'other'
    END) END,
       COUNT(*)
FROM (
    SELECT DISTINCT ct.content_hash, t.key
    FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
) tagged
JOIN locations l ON l.content_hash = tagged.content_hash
LEFT JOIN media_metadata mm ON mm.location_id = l.id
GROUP BY tagged.key, 2;

-- Treat CBZ archives as the existing first-class `comic` media kind so root
-- kind facets can answer comics availability without an extra browse query.
-- The CBZ extension is authoritative even when legacy metadata says `other`.
DROP TRIGGER IF EXISTS kind_counts_location_insert;
DROP TRIGGER IF EXISTS kind_counts_location_delete;
DROP TRIGGER IF EXISTS kind_counts_location_extension_update;
DROP TRIGGER IF EXISTS kind_counts_metadata_insert;
DROP TRIGGER IF EXISTS kind_counts_metadata_update;
DROP TRIGGER IF EXISTS kind_counts_metadata_delete;

DELETE FROM kind_counts;
INSERT INTO kind_counts (kind, files_count)
SELECT CASE
        WHEN lower(l.extension) = '.cbz' THEN 'comic'
        ELSE coalesce(mm.media_kind, CASE
            WHEN lower(l.extension) = '.gif' THEN 'gif'
            WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END)
    END AS kind,
    COUNT(*)
FROM locations l
LEFT JOIN media_metadata mm ON mm.location_id = l.id
GROUP BY kind;

CREATE TRIGGER kind_counts_location_insert
AFTER INSERT ON locations
BEGIN
    INSERT INTO kind_counts (kind, files_count)
    VALUES (
        CASE
            WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
            WHEN lower(NEW.extension) = '.gif' THEN 'gif'
            WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END,
        1
    )
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_location_delete
BEFORE DELETE ON locations
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = CASE
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
END;

CREATE TRIGGER kind_counts_location_extension_update
AFTER UPDATE OF extension ON locations
WHEN OLD.extension IS NOT NEW.extension
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = CASE
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

    INSERT INTO kind_counts (kind, files_count)
    VALUES (
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
    )
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_insert
AFTER INSERT ON media_metadata
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = (
        SELECT CASE
            WHEN lower(extension) = '.cbz' THEN 'comic'
            WHEN lower(extension) = '.gif' THEN 'gif'
            WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END
        FROM locations
        WHERE id = NEW.location_id
    );

    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END, 1
    FROM locations
    WHERE id = NEW.location_id
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_update
AFTER UPDATE OF media_kind ON media_metadata
WHEN OLD.media_kind IS NOT NEW.media_kind
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = (
        SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END
        FROM locations
        WHERE id = NEW.location_id
    );
    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END, 1
    FROM locations
    WHERE id = NEW.location_id
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_delete
AFTER DELETE ON media_metadata
WHEN EXISTS (SELECT 1 FROM locations WHERE id = OLD.location_id)
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = (
        SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END
        FROM locations
        WHERE id = OLD.location_id
    );
    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE
        WHEN lower(extension) = '.cbz' THEN 'comic'
        WHEN lower(extension) = '.gif' THEN 'gif'
        WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
        WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
        ELSE 'other'
    END,
    1
    FROM locations
    WHERE id = OLD.location_id
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

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

