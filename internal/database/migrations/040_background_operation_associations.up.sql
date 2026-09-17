CREATE TABLE background_operation_associations (
    producer_operation_id TEXT NOT NULL,
    auxiliary_kind TEXT NOT NULL CHECK (auxiliary_kind <> ''),
    auxiliary_operation_id TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (producer_operation_id, auxiliary_kind),
    FOREIGN KEY (producer_operation_id) REFERENCES background_operations(id) ON DELETE CASCADE,
    FOREIGN KEY (auxiliary_operation_id) REFERENCES background_operations(id) ON DELETE CASCADE,
    CHECK (producer_operation_id <> auxiliary_operation_id)
);

-- An associated auxiliary may drain before its producer is done accepting work.
-- The Go lifecycle keeps that auxiliary active while the producer is active. Once
-- the producer becomes terminal, finalize any already-drained auxiliaries in the
-- same transaction; auxiliaries with live children finish normally later.
CREATE TRIGGER background_operations_release_associated_auxiliaries_after_terminal
AFTER UPDATE OF status ON background_operations
WHEN OLD.status IN ('pending', 'running')
 AND NEW.status IN ('completed', 'failed', 'canceled')
BEGIN
    UPDATE background_operations
    SET status = CASE
            WHEN progress_failed > 0 THEN 'failed'
            WHEN EXISTS (
                SELECT 1
                FROM background_tasks
                WHERE operation_id = background_operations.id
                  AND status = 'canceled'
                LIMIT 1
            ) THEN 'canceled'
            ELSE 'completed'
        END,
        started_at = COALESCE(
            started_at,
            NEW.finished_at,
            CAST(strftime('%s', 'now') AS INTEGER) * 1000
        ),
        finished_at = COALESCE(
            NEW.finished_at,
            CAST(strftime('%s', 'now') AS INTEGER) * 1000
        ),
        error_code = CASE
            WHEN progress_failed > 0 THEN 'child_task_failed'
            ELSE ''
        END,
        error_message = CASE
            WHEN progress_failed > 0 THEN progress_failed || ' background task(s) failed'
            ELSE ''
        END
    WHERE id IN (
        SELECT auxiliary_operation_id
        FROM background_operation_associations
        WHERE producer_operation_id = NEW.id
    )
      AND status IN ('pending', 'running')
      AND NOT EXISTS (
          SELECT 1
          FROM background_tasks
          WHERE operation_id = background_operations.id
            AND status IN ('pending', 'running')
          LIMIT 1
      )
      AND (
          progress_total = 0
          OR (
              SELECT count(*)
              FROM background_tasks
              WHERE operation_id = background_operations.id
                AND status IN ('completed', 'failed', 'canceled')
          ) >= progress_total
      );
END;
