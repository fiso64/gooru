package database

import (
	"fmt"

	"gooru.local/types"
)

// RehashLocationPreservingTags updates exactly one tracked location to new content
// while preserving the old content and its tags when sibling locations still use it.
func (s *Store) RehashLocationPreservingTags(oldHash, newHash string, newLoc types.LocationInfo) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// The target content must exist before the location can reference it. This is
	// intentionally idempotent because the new hash may already be tracked by a
	// different location.
	if _, err := tx.Exec("INSERT OR IGNORE INTO contents (hash) VALUES (?)", newHash); err != nil {
		return fmt.Errorf("failed to ensure new content exists: %w", err)
	}

	// Tags describe content, but rehash promises to preserve the tags visible on
	// the modified location. Copy rather than move them: if another location still
	// points at oldHash, that unchanged duplicate must keep the same tags too.
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO content_tags (content_hash, tag_id)
		SELECT ?, tag_id FROM content_tags WHERE content_hash = ?`, newHash, oldHash); err != nil {
		return fmt.Errorf("failed to copy tags to rehashed content: %w", err)
	}

	// content_tags triggers refresh caches for locations that already point at the
	// target hash, but this location still points at oldHash while those inserts
	// happen. Recompute its cache in the same UPDATE that moves it so a rehash onto
	// an existing differently-tagged hash cannot leave stale query/display state.
	res, err := tx.Exec(`
		UPDATE locations
		SET content_hash = ?, size_bytes = ?, mod_time = ?, extension = ?,
		    tags_cache = (
			SELECT IFNULL(GROUP_CONCAT(tag_str, ' '), '')
			FROM (
				SELECT CASE WHEN t.value = '' THEN t.key ELSE t.key || ':' || t.value END AS tag_str
				FROM tags t
				JOIN content_tags ct ON t.id = ct.tag_id
				WHERE ct.content_hash = ?
				ORDER BY t.key, t.value
			)
		    )
		WHERE path = ? AND content_hash = ?`,
		newHash, newLoc.Size, newLoc.ModTime, newLoc.Extension, newHash, newLoc.Path, oldHash)
	if err != nil {
		return fmt.Errorf("failed to update location record: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify location update: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("expected to rehash one location at %q, updated %d", newLoc.Path, updated)
	}

	// Metadata follows content identity. Moving this location to newHash must not
	// delete metadata already known for that target hash; orphaned old-hash
	// metadata is removed by the contents foreign-key cascade below.

	// Only remove the old content after the target location moved away and only
	// when no sibling location still references it. Cascades then clean the old
	// tag associations without touching live duplicate locations.
	if _, err := tx.Exec(`
		DELETE FROM contents
		WHERE hash = ?
		  AND NOT EXISTS (SELECT 1 FROM locations WHERE content_hash = ?)`, oldHash, oldHash); err != nil {
		return fmt.Errorf("failed to clean up orphaned old content: %w", err)
	}

	return tx.Commit()
}
