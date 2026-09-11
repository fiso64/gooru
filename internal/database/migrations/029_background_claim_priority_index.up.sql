DROP INDEX background_tasks_schedulable_idx;

-- Claims order ready work by priority before creation time. Keep that order in the
-- index so a large ready backlog does not require SQLite to materialize and sort all
-- eligible rows before selecting one. available_at remains in the index for filtering
-- after the priority-ordered prefix.
CREATE INDEX background_tasks_schedulable_idx
    ON background_tasks (resource_class, status, priority DESC, created_at, id, available_at);
