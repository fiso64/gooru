package database

import (
	"errors"
	"fmt"
	"strings"

	"gooru.local/types"
)

var ErrInvalidSavedSearchOrder = errors.New("invalid saved search order")

// ListSavedSearchesOrdered returns a user's saved searches in their persisted
// manual order. Searches created after the last explicit reorder have no order
// row yet and append deterministically by creation time and ID.
func (s *Store) ListSavedSearchesOrdered(userID string) ([]types.SavedSearch, error) {
	rows, err := s.Query(`
		SELECT ss.id, ss.user_id, ss.name, ss.query, ss.sort, ss."order",
		       strftime('%s', ss.created_at), strftime('%s', ss.updated_at)
		FROM saved_searches ss
		LEFT JOIN saved_search_order sso
		  ON sso.user_id = ss.user_id AND sso.saved_search_id = ss.id
		WHERE ss.user_id = ?
		ORDER BY CASE WHEN sso.position IS NULL THEN 1 ELSE 0 END,
		         sso.position ASC, ss.created_at ASC, ss.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]types.SavedSearch, 0)
	for rows.Next() {
		var item types.SavedSearch
		if err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Query, &item.Sort, &item.Order, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ReorderSavedSearches atomically replaces a user's complete saved-search
// ordering. A permutation is required so stale clients cannot accidentally hide
// or discard searches created by another tab while a drag is in progress.
func (s *Store) ReorderSavedSearches(userID string, ids []string) error {
	tx, err := s.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id FROM saved_searches WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(ids) != len(existing) {
		return fmt.Errorf("%w: expected %d ids, got %d", ErrInvalidSavedSearchOrder, len(existing), len(ids))
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			return fmt.Errorf("%w: unknown saved search id", ErrInvalidSavedSearchOrder)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: duplicate saved search id", ErrInvalidSavedSearchOrder)
		}
		seen[id] = struct{}{}
	}

	if _, err := tx.Exec("DELETE FROM saved_search_order WHERE user_id = ?", userID); err != nil {
		return err
	}

	const columns = 3 // user_id, saved_search_id, position
	batchSize := maxVars / columns
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		placeholders := strings.Repeat("(?, ?, ?),", len(batch)-1) + "(?, ?, ?)"
		args := make([]interface{}, 0, len(batch)*columns)
		for offset, id := range batch {
			args = append(args, userID, id, start+offset)
		}
		if _, err := tx.Exec(
			"INSERT INTO saved_search_order (user_id, saved_search_id, position) VALUES "+placeholders,
			args...,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
