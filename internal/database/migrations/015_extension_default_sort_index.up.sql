-- Extension-filtered library pages use the default added_at DESC, id ASC order.
-- Replace the narrower expression index with a composite index whose leading
-- expression preserves the same equality lookup while also covering page order.
DROP INDEX IF EXISTS idx_locations_extension_lower;
CREATE INDEX IF NOT EXISTS idx_locations_extension_lower_added_at_desc_id_asc
ON locations(lower(extension), added_at DESC, id ASC);
