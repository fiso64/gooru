CREATE TABLE managed_storage_locations (
    location_id INTEGER PRIMARY KEY NOT NULL,
    physical_path TEXT NOT NULL UNIQUE,
    FOREIGN KEY (location_id) REFERENCES locations(id) ON DELETE CASCADE
);
