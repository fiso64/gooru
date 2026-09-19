-- Index each ':'/'_'-delimited completion component once per tag so
-- unqualified suggestions can use the same component-prefix matching as the UI
-- without scanning the tag catalogue for every keystroke.
CREATE TABLE tag_completion_components (
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    component TEXT NOT NULL COLLATE NOCASE,
    PRIMARY KEY (tag_id, component)
);
CREATE INDEX idx_tag_completion_component_prefix
    ON tag_completion_components(component COLLATE NOCASE, tag_id);

INSERT OR IGNORE INTO tag_completion_components(tag_id, component)
WITH RECURSIVE parts(tag_id, rest, component) AS (
    SELECT id, replace(key || ':' || value, ':', '_') || '_', '' FROM tags
    UNION ALL
    SELECT tag_id, substr(rest, instr(rest, '_') + 1), substr(rest, 1, instr(rest, '_') - 1)
    FROM parts WHERE rest != ''
)
SELECT tag_id, component FROM parts WHERE component != '';

CREATE TRIGGER tag_completion_components_insert AFTER INSERT ON tags BEGIN
    INSERT OR IGNORE INTO tag_completion_components(tag_id, component)
    WITH RECURSIVE parts(rest, component) AS (
        SELECT replace(NEW.key || ':' || NEW.value, ':', '_') || '_', ''
        UNION ALL
        SELECT substr(rest, instr(rest, '_') + 1), substr(rest, 1, instr(rest, '_') - 1)
        FROM parts WHERE rest != ''
    )
    SELECT NEW.id, component FROM parts WHERE component != '';
END;

CREATE TRIGGER tag_completion_components_update AFTER UPDATE OF key, value ON tags BEGIN
    DELETE FROM tag_completion_components WHERE tag_id = NEW.id;
    INSERT OR IGNORE INTO tag_completion_components(tag_id, component)
    WITH RECURSIVE parts(rest, component) AS (
        SELECT replace(NEW.key || ':' || NEW.value, ':', '_') || '_', ''
        UNION ALL
        SELECT substr(rest, instr(rest, '_') + 1), substr(rest, 1, instr(rest, '_') - 1)
        FROM parts WHERE rest != ''
    )
    SELECT NEW.id, component FROM parts WHERE component != '';
END;

CREATE TRIGGER tag_completion_components_delete AFTER DELETE ON tags BEGIN
    DELETE FROM tag_completion_components WHERE tag_id = OLD.id;
END;
