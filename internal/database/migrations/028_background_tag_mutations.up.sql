CREATE TABLE background_tag_mutations (
    operation_id TEXT PRIMARY KEY,
    mutation TEXT NOT NULL CHECK (mutation IN ('add', 'set', 'remove')),
    selector_json TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('file_id', 'content_hash')),
    matched_files INTEGER NOT NULL DEFAULT 0 CHECK (matched_files >= 0),
    affected_count INTEGER,
    notifications_json TEXT,
    FOREIGN KEY (operation_id) REFERENCES background_operations(id) ON DELETE CASCADE
);

CREATE TABLE background_tag_mutation_targets (
    operation_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    target_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (operation_id, target_id),
    FOREIGN KEY (operation_id) REFERENCES background_tag_mutations(operation_id) ON DELETE CASCADE
);

CREATE INDEX idx_background_tag_mutation_targets_order
ON background_tag_mutation_targets(operation_id, target_order, target_id);
