-- /tags?counts=true is a bounded popularity query over the maintained key and
-- key/value count summaries. Give each UNION ALL arm the exact requested order
-- so SQLite can merge the two ordered streams and stop at LIMIT instead of
-- sorting every distinct tag in the library.
CREATE INDEX idx_tag_key_counts_rank
    ON tag_key_counts(files_count DESC, key ASC);

CREATE INDEX idx_tags_value_rank
    ON tags(files_count DESC, (key || ':' || value) ASC)
    WHERE value != '' AND files_count > 0;
