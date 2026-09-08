-- Once a logical operation is terminal, child work must not be introduced later.
-- In particular this makes operation cancellation sticky across producer races and
-- restarts instead of allowing a late enqueue to resurrect work beneath a canceled
-- parent.
CREATE TRIGGER background_tasks_require_active_operation_before_insert
BEFORE INSERT ON background_tasks
WHEN NEW.operation_id IS NOT NULL
 AND NOT EXISTS (
    SELECT 1
    FROM background_operations
    WHERE id = NEW.operation_id
      AND status IN ('pending', 'running')
 )
BEGIN
    SELECT RAISE(ABORT, 'background operation is not active');
END;
