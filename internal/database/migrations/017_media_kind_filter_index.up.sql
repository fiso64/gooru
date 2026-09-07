-- `type:` queries compare media kinds case-insensitively. Replace the binary
-- single-column index with an expression index matching that predicate and keep
-- location_id in the index so metadata-backed location matches are covering.
DROP INDEX IF EXISTS idx_media_metadata_kind;
CREATE INDEX idx_media_metadata_kind_lower_location
    ON media_metadata(lower(media_kind), location_id);
