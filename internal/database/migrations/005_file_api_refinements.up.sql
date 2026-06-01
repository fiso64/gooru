CREATE TABLE media_metadata (
    location_id INTEGER PRIMARY KEY,
    media_kind TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    image_width INTEGER,
    image_height INTEGER,
    video_width INTEGER,
    video_height INTEGER,
    duration_seconds REAL,
    frame_count INTEGER,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (location_id) REFERENCES locations(id) ON DELETE CASCADE
);

CREATE INDEX idx_locations_mod_time ON locations(mod_time);
CREATE INDEX idx_locations_size_bytes ON locations(size_bytes);
CREATE INDEX idx_locations_extension ON locations(extension);
CREATE INDEX idx_locations_lower_path ON locations(lower(path));
CREATE INDEX idx_media_metadata_kind ON media_metadata(media_kind);

CREATE TABLE saved_searches (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    query TEXT NOT NULL,
    sort TEXT NOT NULL,
    "order" TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_saved_searches_user_name ON saved_searches(user_id, name COLLATE NOCASE);
CREATE INDEX idx_saved_searches_user_updated ON saved_searches(user_id, updated_at DESC);
