DROP INDEX IF EXISTS idx_locations_extension_lower_added_at_desc_id_asc;
CREATE INDEX IF NOT EXISTS idx_locations_extension_lower ON locations(lower(extension));
