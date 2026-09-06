DROP INDEX IF EXISTS idx_content_tags_tag_id_content_hash;
CREATE INDEX IF NOT EXISTS idx_content_tags_tag_id ON content_tags(tag_id);
