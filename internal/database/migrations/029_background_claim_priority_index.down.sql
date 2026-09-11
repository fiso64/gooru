DROP INDEX background_tasks_schedulable_idx;

CREATE INDEX background_tasks_schedulable_idx
    ON background_tasks (resource_class, status, available_at, priority DESC, created_at, id);
