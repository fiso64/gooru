-- Keep basename-only filename search indexed for interactive queries over large libraries.
CREATE VIRTUAL TABLE location_filenames USING fts5(
    filename,
    tokenize='trigram'
);

-- Backfill existing locations once at migration time. Path splitting belongs on
-- writes/upgrades, not on every filename search.
WITH RECURSIVE path_parts(id, rest, part) AS (
    SELECT id, replace(path, char(92), '/') || '/', '' FROM locations
    UNION ALL
    SELECT id,
           substr(rest, instr(rest, '/') + 1),
           substr(rest, 1, instr(rest, '/') - 1)
    FROM path_parts
    WHERE rest <> ''
)
INSERT INTO location_filenames(rowid, filename)
SELECT id, part
FROM path_parts
WHERE rest = '';

CREATE TRIGGER sync_location_filename_after_insert
AFTER INSERT ON locations
BEGIN
    INSERT INTO location_filenames(rowid, filename)
    SELECT NEW.id, part
    FROM (
        WITH RECURSIVE path_parts(rest, part) AS (
            SELECT replace(NEW.path, char(92), '/') || '/', ''
            UNION ALL
            SELECT substr(rest, instr(rest, '/') + 1),
                   substr(rest, 1, instr(rest, '/') - 1)
            FROM path_parts
            WHERE rest <> ''
        )
        SELECT rest, part FROM path_parts
    )
    WHERE rest = '';
END;

CREATE TRIGGER sync_location_filename_after_path_update
AFTER UPDATE OF path ON locations
BEGIN
    DELETE FROM location_filenames WHERE rowid = OLD.id;
    INSERT INTO location_filenames(rowid, filename)
    SELECT NEW.id, part
    FROM (
        WITH RECURSIVE path_parts(rest, part) AS (
            SELECT replace(NEW.path, char(92), '/') || '/', ''
            UNION ALL
            SELECT substr(rest, instr(rest, '/') + 1),
                   substr(rest, 1, instr(rest, '/') - 1)
            FROM path_parts
            WHERE rest <> ''
        )
        SELECT rest, part FROM path_parts
    )
    WHERE rest = '';
END;

CREATE TRIGGER sync_location_filename_after_delete
AFTER DELETE ON locations
BEGIN
    DELETE FROM location_filenames WHERE rowid = OLD.id;
END;
