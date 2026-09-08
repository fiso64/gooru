-- Once a logical operation is canceled, child work must not be introduced later.
-- This makes cancellation sticky across producer races and restarts instead of
-- allowing a late enqueue to resurrect work beneath a canceled parent. Keep this
-- guard cancellation-specific: completed/failed operation enqueue semantics are a
-- separate lifecycle policy and should not be changed by this slice.
CREATE TRIGGER background_tasks_reject_canceled_operation_before_insert
BEFORE INSERT ON background_tasks
WHEN NEW.operation_id IS NOT NULL
 AND EXISTS (
    SELECT 1
    FROM background_operations
    WHERE id = NEW.operation_id
      AND status = 'canceled'
 )
BEGIN
    SELECT RAISE(ABORT, 'background operation is canceled');
END;
