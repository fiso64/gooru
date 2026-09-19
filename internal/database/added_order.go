package database

import (
	"fmt"
	"strings"

	"gooru.local/types"
)

// BatchUpsertLocationsWithAddedOrder is the upload-aware location upsert path.
// locations.added_at is stored as Unix milliseconds by migration 041, while
// added_order is a non-time secondary ordering key for equal added timestamps.
// Existing location identity keeps its original added timestamp/order on path
// conflict, matching BatchUpsertLocations' historical insertion-time semantics.
func (s *Store) BatchUpsertLocationsWithAddedOrder(q Querier, locations map[string]types.LocationInfo, addedOrderByPath map[string]int64) error {
	const columns = 7 // content_hash, path, size_bytes, mod_time, added_at, added_order, extension
	const rowPlaceholders = "('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?, ?, ?)"
	return s.batchUpsertLocations(
		q,
		locations,
		"public_id, content_hash, path, size_bytes, mod_time, added_at, added_order, extension",
		rowPlaceholders,
		columns,
		func(args []any, loc types.LocationInfo) {
			args[0], args[1], args[2] = loc.Hash, loc.Path, loc.Size
			args[3], args[4], args[5], args[6] = loc.ModTime, loc.AddedAt, addedOrderByPath[loc.Path], loc.Extension
		},
	)
}

func (s *Store) addedOrderCursorClause(cursor *types.PageCursor, order string) (string, []interface{}, error) {
	if cursor == nil {
		return "", nil, nil
	}
	var addedAt, addedOrder int64
	if err := s.QueryRow(`SELECT added_at, added_order FROM locations WHERE id = ?`, cursor.ID).Scan(&addedAt, &addedOrder); err != nil {
		return "", nil, err
	}
	comparison := ">"
	if strings.EqualFold(order, "desc") {
		comparison = "<"
	}
	clause := fmt.Sprintf(`AND (
		l.added_at %s ? OR
		(l.added_at = ? AND l.added_order %s ?) OR
		(l.added_at = ? AND l.added_order = ? AND l.id > ?)
	)`, comparison, comparison)
	return clause, []interface{}{addedAt, addedAt, addedOrder, addedAt, addedOrder, cursor.ID}, nil
}

func addedOrderSortClause(order string) string {
	direction := sortOrder(order)
	return fmt.Sprintf("l.added_at %s, l.added_order %s, l.id ASC", direction, direction)
}
