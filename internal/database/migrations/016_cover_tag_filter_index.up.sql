-- Exact tag filters resolve a tag id first and then need each matching content
-- hash. Cover both columns so large tag buckets do not require a table lookup
-- for every content_tags row after probing by tag_id.
DROP INDEX IF EXISTS idx_content_tags_tag_id;
CREATE INDEX IF NOT EXISTS idx_content_tags_tag_id_content_hash
ON content_tags(tag_id, content_hash);
