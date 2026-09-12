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

	res, err := tx.Exec(`
		UPDATE locations
		SET content_hash = ?, size_bytes = ?, mod_time = ?, extension = ?
		WHERE path = ? AND content_hash = ?`,
		newHash, newLoc.Size, newLoc.ModTime, newLoc.Extension, newLoc.Path, oldHash)
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

	// Media metadata is derived from the file bytes. A successful rehash proves
	// those bytes changed, so keeping the previous MIME/dimensions/duration would
	// expose stale data. Keep the location row (and therefore managed-storage
	// identity) but invalidate its derived metadata so normal extraction can
	// repopulate it from the new content later.
	if _, err := tx.Exec(`
		DELETE FROM media_metadata
		WHERE location_id IN (
			SELECT id FROM locations WHERE path = ? AND content_hash = ?
		)`, newLoc.Path, newHash); err != nil {
		return fmt.Errorf("failed to invalidate stale media metadata: %w", err)
	}

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
