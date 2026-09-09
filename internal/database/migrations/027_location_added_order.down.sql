DROP INDEX IF EXISTS idx_locations_added_order_desc_id_asc;
DROP INDEX IF EXISTS idx_locations_added_order_id;
DROP TRIGGER IF EXISTS set_location_added_order_on_insert;
ALTER TABLE locations DROP COLUMN added_order;
