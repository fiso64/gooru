-- A positive progress_total is a declared child-work total. Producers may attach
-- additional durable children incrementally, including after a terminal operation
-- reopens. Keep that declared total from falling below the number of attached tasks
-- so user-facing progress cannot become 2/1 (or worse). A zero total deliberately
-- remains zero because it means the total is unknown.
--
-- Persist the attached count so enqueue remains O(1) for large fan-out operations.
-- Existing operations are backfilled once during migration; subsequent successful
-- child inserts increment the counter in the same transaction.
ALTER TABLE background_operations
    ADD COLUMN attached_task_count INTEGER NOT NULL DEFAULT 0 CHECK (attached_task_count >= 0);

UPDATE background_operations
SET attached_task_count = (
    SELECT count(*)
    FROM background_tasks
    WHERE background_tasks.operation_id = background_operations.id
);

-- Replace the 033 trigger rather than layering another terminal-only trigger so the
-- same INSERT transaction owns all three invariants: attached count, positive total,
-- and immediate terminal-parent reopen.
DROP TRIGGER IF EXISTS background_tasks_reopen_terminal_operation_after_insert;

CREATE TRIGGER background_tasks_reopen_terminal_operation_after_insert
AFTER INSERT ON background_tasks
WHEN NEW.operation_id IS NOT NULL
BEGIN
    UPDATE background_operations
    SET attached_task_count = attached_task_count + 1,
        progress_total = CASE
            WHEN progress_total > 0 AND progress_total < attached_task_count + 1
                THEN attached_task_count + 1
            ELSE progress_total
        END,
        status = CASE
            WHEN status IN ('completed', 'failed') THEN 'pending'
            ELSE status
        END,
        finished_at = CASE
            WHEN status IN ('completed', 'failed') THEN NULL
            ELSE finished_at
        END,
        error_code = CASE
            WHEN status IN ('completed', 'failed') THEN ''
            ELSE error_code
        END,
        error_message = CASE
            WHEN status IN ('completed', 'failed') THEN ''
            ELSE error_message
        END,
        result_json = CASE
            WHEN status IN ('completed', 'failed') THEN ''
            ELSE result_json
        END
    WHERE id = NEW.operation_id;
END;
