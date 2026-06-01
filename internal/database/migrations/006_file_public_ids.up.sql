ALTER TABLE locations ADD COLUMN public_id TEXT;

UPDATE locations
SET public_id = 'file_' || lower(hex(randomblob(16)))
WHERE public_id IS NULL OR public_id = '';

CREATE UNIQUE INDEX idx_locations_public_id ON locations(public_id);
