-- This drops the entire schema, effectively reverting to an empty database.
DROP TRIGGER IF EXISTS cleanup_orphan_content_on_delete;
DROP TRIGGER IF EXISTS cleanup_orphan_tags_on_delete;
DROP TRIGGER IF EXISTS update_tags_cache_on_delete;
DROP TRIGGER IF EXISTS update_tags_cache_on_insert;
DROP TRIGGER IF EXISTS populate_tags_cache_on_location_insert;
DROP INDEX IF EXISTS idx_locations_extension_lower;
DROP INDEX IF EXISTS idx_content_tags_tag_id;
DROP INDEX IF EXISTS idx_locations_content_hash;
DROP INDEX IF EXISTS idx_locations_path;
DROP TABLE IF EXISTS content_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS contents;
DROP TABLE IF EXISTS meta;