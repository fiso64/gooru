-- Persisting a new child beneath a previously completed/failed operation means the
-- logical operation is no longer terminal, even if the scheduler has not claimed the
-- child yet. Reopen the parent in the same INSERT transaction so readers can never
-- observe terminal state or a prior epoch's result/error while durable pending work
-- already exists. Cancellation remains sticky via the BEFORE INSERT trigger from 024.
CREATE TRIGGER background_tasks_reopen_terminal_operation_after_insert
AFTER INSERT ON background_tasks
WHEN NEW.operation_id IS NOT NULL
BEGIN
    UPDATE background_operations
    SET status = 'pending',
        finished_at = NULL,
        error_code = '',
        error_message = '',
        result_json = ''
    WHERE id = NEW.operation_id
      AND status IN ('completed', 'failed');
END;
