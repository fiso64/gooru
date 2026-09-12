CREATE INDEX background_operations_pending_kind_idx
    ON background_operations (kind)
    WHERE status = 'pending';
