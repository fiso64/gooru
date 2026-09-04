DROP INDEX IF EXISTS idx_locations_added_at_id;
DROP TRIGGER IF EXISTS set_location_added_at_on_insert;
ALTER TABLE locations DROP COLUMN added_at;
