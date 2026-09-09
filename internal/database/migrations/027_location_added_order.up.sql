-- Keep user-facing added_at in Unix seconds while maintaining a separate,
-- higher-precision key for deterministic library ordering. Existing rows retain
-- their current added_at ordering and ID tie-break semantics.
ALTER TABLE locations ADD COLUMN added_order INTEGER NOT NULL DEFAULT 0;

UPDATE locations
SET added_order = added_at * 1000000
WHERE added_order = 0;

-- Legacy callers may omit added_order. Populate a compatible key without
-- changing the public added_at contract.
CREATE TRIGGER set_location_added_order_on_insert
    AFTER INSERT ON locations
    WHEN NEW.added_order = 0
    BEGIN
        UPDATE locations
        SET added_order = CASE
            WHEN NEW.added_at != 0 THEN NEW.added_at * 1000000
            ELSE CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)
        END
        WHERE id = NEW.id;
    END;

CREATE INDEX idx_locations_added_order_id
    ON locations(added_order, id);
CREATE INDEX idx_locations_added_order_desc_id_asc
    ON locations(added_order DESC, id ASC);
