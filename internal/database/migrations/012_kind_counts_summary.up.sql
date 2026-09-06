-- Root-library kind facets are requested on the interactive browse path. Keep
-- the unfiltered aggregate incrementally so reads do not scan every location.
CREATE TABLE kind_counts (
    kind TEXT PRIMARY KEY,
    files_count INTEGER NOT NULL CHECK (files_count >= 0)
);

INSERT INTO kind_counts (kind, files_count)
SELECT coalesce(mm.media_kind, CASE
        WHEN lower(l.extension) = '.gif' THEN 'gif'
        WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
        WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
        ELSE 'other'
    END) AS kind,
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
    WHERE kind = coalesce(
        (SELECT media_kind FROM media_metadata WHERE location_id = OLD.id),
        CASE
            WHEN lower(OLD.extension) = '.gif' THEN 'gif'
            WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END
    );
END;

CREATE TRIGGER kind_counts_location_extension_update
AFTER UPDATE OF extension ON locations
WHEN OLD.extension IS NOT NEW.extension
 AND NOT EXISTS (SELECT 1 FROM media_metadata WHERE location_id = NEW.id)
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = CASE
        WHEN lower(OLD.extension) = '.gif' THEN 'gif'
        WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
        WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
        ELSE 'other'
    END;

    INSERT INTO kind_counts (kind, files_count)
    VALUES (
        CASE
            WHEN lower(NEW.extension) = '.gif' THEN 'gif'
            WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
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
            WHEN lower(extension) = '.gif' THEN 'gif'
            WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END
        FROM locations
        WHERE id = NEW.location_id
    );

    INSERT INTO kind_counts (kind, files_count)
    VALUES (NEW.media_kind, 1)
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_update
AFTER UPDATE OF media_kind ON media_metadata
WHEN OLD.media_kind IS NOT NEW.media_kind
BEGIN
    UPDATE kind_counts SET files_count = files_count - 1 WHERE kind = OLD.media_kind;
    INSERT INTO kind_counts (kind, files_count)
    VALUES (NEW.media_kind, 1)
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_delete
AFTER DELETE ON media_metadata
WHEN EXISTS (SELECT 1 FROM locations WHERE id = OLD.location_id)
BEGIN
    UPDATE kind_counts SET files_count = files_count - 1 WHERE kind = OLD.media_kind;
    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE
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
