-- Extension filters compile to lower(extension) = lower(?). Keep that common
-- library/count path indexed without changing the case-insensitive query contract.
CREATE INDEX IF NOT EXISTS idx_locations_extension_lower ON locations(lower(extension));
