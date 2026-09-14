ALTER TABLE background_operations
    ADD COLUMN producer_claimed INTEGER NOT NULL DEFAULT 0 CHECK (producer_claimed IN (0, 1));
