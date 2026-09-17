CREATE INDEX background_operations_active_kind_created_idx
    ON background_operations (kind, created_at DESC, id DESC)
    WHERE status IN ('pending', 'running');
