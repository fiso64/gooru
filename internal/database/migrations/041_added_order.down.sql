DROP INDEX IF EXISTS idx_locations_added_at_order_desc_id_asc;
DROP INDEX IF EXISTS idx_locations_added_at_order_id;
DROP TRIGGER IF EXISTS set_location_added_at_on_insert;

UPDATE locations
SET added_at = CAST(added_at / 1000 AS INTEGER);

ALTER TABLE locations DROP COLUMN added_order;

CREATE TRIGGER set_location_added_at_on_insert
AFTER INSERT ON locations
WHEN NEW.added_at = 0
BEGIN
    UPDATE locations
    SET added_at = CAST(strftime('%s','now') AS INTEGER)
    WHERE id = NEW.id;
END;

CREATE INDEX idx_locations_added_at_id
ON locations(added_at, id);

CREATE INDEX idx_locations_added_at_desc_id_asc
ON locations(added_at DESC, id ASC);
