-- Keep added_at as an honest timestamp, but raise its storage precision from
-- Unix seconds to Unix milliseconds. added_order is a non-time secondary key
-- used to preserve deterministic ordering within one upload batch.
DROP INDEX IF EXISTS idx_locations_added_at_desc_id_asc;
DROP INDEX IF EXISTS idx_locations_added_at_id;
DROP TRIGGER IF EXISTS set_location_added_at_on_insert;

UPDATE locations
SET added_at = added_at * 1000;

ALTER TABLE locations ADD COLUMN added_order INTEGER NOT NULL DEFAULT 0;

-- Compatibility: callers predating this migration may still provide Unix
-- seconds. Normalize those values on insert while allowing new millisecond
-- callers through unchanged. A zero value keeps the historical "assign now"
-- behavior, now at millisecond precision.
CREATE TRIGGER set_location_added_at_on_insert
AFTER INSERT ON locations
WHEN NEW.added_at < 100000000000
BEGIN
    UPDATE locations
    SET added_at = CASE
        WHEN NEW.added_at = 0 THEN CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)
        ELSE NEW.added_at * 1000
    END
    WHERE id = NEW.id;
END;

CREATE INDEX idx_locations_added_at_order_id
ON locations(added_at ASC, added_order ASC, id ASC);

CREATE INDEX idx_locations_added_at_order_desc_id_asc
ON locations(added_at DESC, added_order DESC, id ASC);
