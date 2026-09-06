DROP INDEX IF EXISTS idx_media_metadata_kind_lower_location;
CREATE INDEX idx_media_metadata_kind ON media_metadata(media_kind);
