-- added_at is Gooru-owned ordering metadata, independent of filesystem modtime.
-- Existing rows preserve the closest available historical signal by backfilling
-- from mod_time. A zero default keeps ALTER TABLE compatible with existing
-- SQLite databases; the trigger supplies the insertion time for legacy callers
-- that do not yet pass an explicit added_at value.
ALTER TABLE locations ADD COLUMN added_at INTEGER NOT NULL DEFAULT 0;

UPDATE locations
SET added_at = mod_time
WHERE added_at = 0;

CREATE TRIGGER set_location_added_at_on_insert
    AFTER INSERT ON locations
    WHEN NEW.added_at = 0
    BEGIN
        UPDATE locations
        SET added_at = CAST(strftime('%s', 'now') AS INTEGER)
        WHERE id = NEW.id;
    END;

CREATE INDEX idx_locations_added_at_id ON locations(added_at, id);
