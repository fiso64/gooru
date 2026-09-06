-- The WebUI's default library order is newest-first with location ID as the
-- stable ascending tie-breaker. The existing (added_at, id) index can satisfy
-- added_at ASC / id ASC, but reversing it also reverses id and forces SQLite to
-- sort large equal-timestamp batches. Keep an exact mixed-direction index for
-- the hot default browse path.
CREATE INDEX idx_locations_added_at_desc_id_asc
    ON locations(added_at DESC, id ASC);
