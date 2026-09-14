CREATE TRIGGER invalidate_media_metadata_on_location_content_change
AFTER UPDATE OF content_hash ON locations
WHEN OLD.content_hash IS NOT NEW.content_hash
BEGIN
    DELETE FROM media_metadata WHERE location_id = NEW.id;
END;
