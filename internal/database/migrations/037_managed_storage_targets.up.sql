CREATE TABLE managed_storage_targets (
    target_id TEXT PRIMARY KEY NOT NULL,
    root_path TEXT NOT NULL,
    path_prefix TEXT NOT NULL,
    path_upper TEXT NOT NULL
);

CREATE TABLE managed_storage_target_locations (
    target_id TEXT NOT NULL,
    location_id INTEGER NOT NULL,
    PRIMARY KEY (target_id, location_id),
    FOREIGN KEY (target_id) REFERENCES managed_storage_targets(target_id) ON DELETE CASCADE,
    FOREIGN KEY (location_id) REFERENCES locations(id) ON DELETE CASCADE
);

CREATE INDEX managed_storage_target_locations_location_idx
    ON managed_storage_target_locations(location_id, target_id);

CREATE TRIGGER managed_storage_target_locations_after_location_insert
AFTER INSERT ON locations
BEGIN
    INSERT OR IGNORE INTO managed_storage_target_locations (target_id, location_id)
    SELECT target_id, NEW.id
    FROM managed_storage_targets
    WHERE NEW.path = root_path
       OR (NEW.path >= path_prefix AND NEW.path < path_upper);
END;

CREATE TRIGGER managed_storage_target_locations_after_location_path_update
AFTER UPDATE OF path ON locations
BEGIN
    DELETE FROM managed_storage_target_locations WHERE location_id = NEW.id;
    INSERT OR IGNORE INTO managed_storage_target_locations (target_id, location_id)
    SELECT target_id, NEW.id
    FROM managed_storage_targets
    WHERE NEW.path = root_path
       OR (NEW.path >= path_prefix AND NEW.path < path_upper);
END;
