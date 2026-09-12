DROP TRIGGER IF EXISTS background_tasks_reopen_terminal_operation_after_insert;

-- Remove the schema state introduced by migration 034 so a down/up cycle can
-- reapply the migration without a duplicate-column failure.
ALTER TABLE background_operations DROP COLUMN attached_task_count;

-- Restore the terminal-reopen behavior introduced by migration 033 when rolling
-- this migration back. The older trigger intentionally does not adjust progress_total.
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