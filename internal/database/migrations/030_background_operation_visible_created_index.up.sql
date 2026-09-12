CREATE INDEX background_operations_visible_created_idx
    ON background_operations (visible, created_at DESC, id DESC);
