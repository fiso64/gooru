-- This migration defines the initial "v1" schema for the Gooru database.

CREATE TABLE meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- The db_version tracks breaking changes in application logic (e.g. hashing algorithm),
-- not just schema changes.
INSERT INTO meta (key, value) VALUES ('db_version', '1');

CREATE TABLE contents (
    hash TEXT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE locations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content_hash TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    size_bytes INTEGER NOT NULL,
    mod_time INTEGER NOT NULL,
    extension TEXT NOT NULL,
    tags_cache TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (content_hash) REFERENCES contents(hash) ON DELETE CASCADE
);

CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT NOT NULL COLLATE NOCASE,
    value TEXT NOT NULL COLLATE NOCASE,
    UNIQUE(key, value)
);

CREATE TABLE content_tags (
    content_hash TEXT NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (content_hash, tag_id),
    FOREIGN KEY (content_hash) REFERENCES contents(hash) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE INDEX idx_locations_path ON locations(path);
CREATE INDEX idx_locations_content_hash ON locations(content_hash);
CREATE INDEX idx_content_tags_tag_id ON content_tags(tag_id);
CREATE INDEX idx_locations_extension_lower ON locations(lower(extension));

-- TRIGGERS FOR MAINTAINING tags_cache
CREATE TRIGGER populate_tags_cache_on_location_insert
    AFTER INSERT ON locations
    BEGIN
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = NEW.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE id = NEW.id;
    END;

CREATE TRIGGER update_tags_cache_on_insert
    AFTER INSERT ON content_tags
    BEGIN
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = NEW.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE content_hash = NEW.content_hash;
    END;

CREATE TRIGGER update_tags_cache_on_delete
    AFTER DELETE ON content_tags
    BEGIN
        UPDATE locations
        SET tags_cache = (
            SELECT IFNULL(GROUP_CONCAT(tag_str), '')
            FROM (
                     SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
                     FROM tags t
                              JOIN content_tags ct ON t.id = ct.tag_id
                     WHERE ct.content_hash = OLD.content_hash
                     ORDER BY t.key, t.value
                 )
        )
        WHERE content_hash = OLD.content_hash;
    END;

-- TRIGGER FOR CLEANING UP ORPHANED TAGS
CREATE TRIGGER cleanup_orphan_tags_on_delete
    AFTER DELETE ON content_tags
    BEGIN
        DELETE FROM tags
        WHERE id = OLD.tag_id
          AND NOT EXISTS (
            SELECT 1 FROM content_tags WHERE tag_id = OLD.tag_id
        );
    END;

-- TRIGGER FOR CLEANING UP ORPHANED CONTENT
CREATE TRIGGER cleanup_orphan_content_on_delete
    AFTER DELETE ON locations
    BEGIN
        DELETE FROM contents
        WHERE hash = OLD.content_hash
          AND NOT EXISTS (
            SELECT 1 FROM locations WHERE content_hash = OLD.content_hash
        );
    END;