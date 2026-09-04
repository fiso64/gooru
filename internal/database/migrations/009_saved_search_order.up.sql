CREATE TABLE saved_search_order (
    user_id TEXT NOT NULL,
    saved_search_id TEXT NOT NULL,
    position INTEGER NOT NULL CHECK(position >= 0),
    PRIMARY KEY (user_id, saved_search_id),
    UNIQUE (user_id, position),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (saved_search_id) REFERENCES saved_searches(id) ON DELETE CASCADE
);

INSERT INTO saved_search_order (user_id, saved_search_id, position)
SELECT user_id,
       id,
       ROW_NUMBER() OVER (
           PARTITION BY user_id
           ORDER BY updated_at DESC, name COLLATE NOCASE ASC, id ASC
       ) - 1
FROM saved_searches;
