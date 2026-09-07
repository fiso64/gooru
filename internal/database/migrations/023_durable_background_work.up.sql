CREATE TABLE background_operations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    visible INTEGER NOT NULL DEFAULT 1 CHECK (visible IN (0, 1)),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'canceled')),
    progress_total INTEGER NOT NULL DEFAULT 0 CHECK (progress_total >= 0),
    progress_completed INTEGER NOT NULL DEFAULT 0 CHECK (progress_completed >= 0),
    progress_failed INTEGER NOT NULL DEFAULT 0 CHECK (progress_failed >= 0),
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    finished_at INTEGER,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX background_operations_visible_status_created_idx
    ON background_operations (visible, status, created_at DESC, id DESC);

CREATE TABLE background_tasks (
    id TEXT PRIMARY KEY,
    operation_id TEXT REFERENCES background_operations(id) ON DELETE SET NULL,
    dedupe_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    subject_kind TEXT NOT NULL DEFAULT '',
    subject_id TEXT NOT NULL DEFAULT '',
    input_key TEXT NOT NULL DEFAULT '',
    resource_class TEXT NOT NULL DEFAULT 'default',
    priority INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'canceled')),
    available_at INTEGER NOT NULL,
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at INTEGER,
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    finished_at INTEGER,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error_message TEXT NOT NULL DEFAULT ''
);

-- A dedupe key identifies work that must not execute concurrently. Terminal history is
-- intentionally allowed to reuse the same key so processors can be rerun after input,
-- configuration, implementation-version, or requirement changes.
CREATE UNIQUE INDEX background_tasks_active_dedupe_idx
    ON background_tasks (dedupe_key)
    WHERE status IN ('pending', 'running');
CREATE INDEX background_tasks_schedulable_idx
    ON background_tasks (resource_class, status, available_at, priority DESC, created_at, id);
CREATE INDEX background_tasks_operation_idx
    ON background_tasks (operation_id, status, created_at, id);
CREATE INDEX background_tasks_lease_idx
    ON background_tasks (status, lease_expires_at)
    WHERE status = 'running';
CREATE INDEX background_tasks_subject_idx
    ON background_tasks (subject_kind, subject_id, kind, status);

CREATE TABLE background_task_attempts (
    task_id TEXT NOT NULL REFERENCES background_tasks(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    worker_id TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    finished_at INTEGER,
    outcome TEXT NOT NULL DEFAULT 'running' CHECK (outcome IN ('running', 'completed', 'failed', 'canceled', 'abandoned')),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (task_id, attempt_number)
);

CREATE INDEX background_task_attempts_started_idx
    ON background_task_attempts (started_at DESC, task_id, attempt_number);
