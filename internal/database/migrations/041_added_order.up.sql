-- Keep added_at as an honest timestamp, but raise its storage precision from
-- Unix seconds to Unix milliseconds. added_order is a non-time secondary key
-- used to preserve deterministic ordering within one upload batch.
DROP INDEX IF EXISTS idx_locations_added_at_desc_id_asc;
DROP INDEX IF EXISTS idx_locations_added_at_id;
DROP INDEX IF EXISTS idx_locations_extension_lower_added_at_desc_id_asc;
DROP TRIGGER IF EXISTS set_location_added_at_on_insert;

UPDATE locations
SET added_at = added_at * 1000;

ALTER TABLE locations ADD COLUMN added_order INTEGER NOT NULL DEFAULT 0;

-- After this migration, added_at is unambiguously Unix milliseconds.
-- Only the zero sentinel needs database-side normalization; trying to infer
-- seconds from magnitude would corrupt legitimate early-epoch millisecond
-- timestamps used by modified-time ordering.
CREATE TRIGGER set_location_added_at_on_insert
AFTER INSERT ON locations
WHEN NEW.added_at = 0
BEGIN
    UPDATE locations
    SET added_at = CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)
    WHERE id = NEW.id;
END;

CREATE INDEX idx_locations_added_at_order_id
ON locations(added_at ASC, added_order ASC, id ASC);

CREATE INDEX idx_locations_added_at_order_desc_id_asc
ON locations(added_at DESC, added_order DESC, id ASC);

CREATE INDEX idx_locations_extension_lower_added_at_order_desc_id_asc
ON locations(lower(extension), added_at DESC, added_order DESC, id ASC);
