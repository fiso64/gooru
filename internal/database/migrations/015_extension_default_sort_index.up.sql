-- Extension-filtered library pages use the default added_at DESC, id ASC order.
-- Cover both the case-insensitive equality predicate and that order so large
-- extension buckets can be paged directly from the index without a temp sort.
CREATE INDEX IF NOT EXISTS idx_locations_extension_lower_added_at_desc_id_asc
ON locations(lower(extension), added_at DESC, id ASC);
