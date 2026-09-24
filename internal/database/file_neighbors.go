package database

import (
	"database/sql"
	"fmt"
	"strings"

	"gooru.local/types"
)

// GetFilesInfoByLocationQueryAround returns nearest-first neighbors in the exact
// order used by the listing. Unlike offset paging it never depends on loaded
// pages, and the reversed lookup reverses *every* ordering key (including ID).
func (s *Store) GetFilesInfoByLocationQueryAround(query string, args []interface{}, id int64, sort, order string, count int) (before, after []types.FileInfo, err error) {
	if count < 1 || count > 20 {
		return nil, nil, fmt.Errorf("neighbor count must be between 1 and 20")
	}
	if query == "" {
		query = "SELECT id FROM locations"
	}
	var present bool
	membership := fmt.Sprintf(`WITH result_locations(id) AS (%s) SELECT EXISTS(SELECT 1 FROM result_locations WHERE id = ?)`, query)
	if err = s.QueryRow(membership, append(append([]interface{}{}, args...), id)...).Scan(&present); err != nil {
		return nil, nil, err
	}
	if !present {
		return nil, nil, sql.ErrNoRows
	}

	forward, forwardArgs, err := s.fileCursorClause(&types.PageCursor{ID: id}, sort, order)
	if err != nil {
		return nil, nil, err
	}
	reverse, reverseArgs, err := s.fileReverseCursorClause(id, sort, order)
	if err != nil {
		return nil, nil, err
	}
	lookup := func(clause string, cursorArgs []interface{}, backwards bool, limit int) ([]types.FileInfo, error) {
		sortClause := fileSortClause(sort, order)
		if backwards {
			sortClause = reverseFileSortClause(sort, order)
		}
		sqlText := fmt.Sprintf(`WITH result_locations(id) AS (%s)
            SELECT %s FROM locations l JOIN result_locations rl ON l.id = rl.id
            LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
            WHERE 1=1 %s ORDER BY %s LIMIT ?`, query, fileInfoColumns(), clause, sortClause)
		params := append(append(append([]interface{}{}, args...), cursorArgs...), limit)
		return s.scanFileInfos(sqlText, params...)
	}
	after, err = lookup(forward, forwardArgs, false, count)
	if err != nil {
		return nil, nil, err
	}
	before, err = lookup(reverse, reverseArgs, true, count)
	if err != nil {
		return nil, nil, err
	}
	// Global wraparound: fill from the opposite end of the *filtered* sequence.
	// Avoid repeating the anchor or a neighbor for listings smaller than the window.
	fill := func(items []types.FileInfo, backwards bool) ([]types.FileInfo, error) {
		if len(items) == count {
			return items, nil
		}
		edge, err := lookup("", nil, backwards, count+1)
		if err != nil {
			return nil, err
		}
		seen := map[int64]bool{id: true}
		for _, file := range items {
			seen[file.ID] = true
		}
		for _, file := range edge {
			if !seen[file.ID] {
				items = append(items, file)
				seen[file.ID] = true
				if len(items) == count {
					break
				}
			}
		}
		return items, nil
	}
	after, err = fill(after, false)
	if err != nil {
		return nil, nil, err
	}
	before, err = fill(before, true)
	return before, after, err
}

func reverseFileSortClause(sort, order string) string {
	reversed := "desc"
	if strings.EqualFold(order, "desc") {
		reversed = "asc"
	}
	direction := sortOrder(reversed)
	if sort == "added" {
		return fmt.Sprintf("l.added_at %s, l.added_order %s, l.id DESC", direction, direction)
	}
	return fmt.Sprintf("%s %s, l.id DESC", fileSortExpression(sort), direction)
}

func (s *Store) fileReverseCursorClause(id int64, sort, order string) (string, []interface{}, error) {
	compare := "<"
	if strings.EqualFold(order, "desc") {
		compare = ">"
	}
	if sort == "added" {
		var timestamp, secondary int64
		if err := s.QueryRow(`SELECT added_at, added_order FROM locations WHERE id = ?`, id).Scan(&timestamp, &secondary); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf(`AND (l.added_at %s ? OR (l.added_at = ? AND l.added_order %s ?) OR (l.added_at = ? AND l.added_order = ? AND l.id < ?))`, compare, compare), []interface{}{timestamp, timestamp, secondary, timestamp, secondary, id}, nil
	}
	key, err := s.cursorKeyForLocation(id, sort)
	if err != nil {
		return "", nil, err
	}
	expr := fileSortExpression(sort)
	return fmt.Sprintf(`AND (%s %s ? OR (%s = ? AND l.id < ?))`, expr, compare, expr), []interface{}{key, key, id}, nil
}
