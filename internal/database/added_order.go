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
	if len(locations) == 0 {
		return nil
	}
	const columns = 7 // content_hash, path, size_bytes, mod_time, added_at, added_order, extension
	batchSize := maxVars / columns

	locs := make([]types.LocationInfo, 0, len(locations))
	for _, loc := range locations {
		locs = append(locs, loc)
	}

	for i := 0; i < len(locs); i += batchSize {
		end := i + batchSize
		if end > len(locs) {
			end = len(locs)
		}
		batch := locs[i:end]

		placeholders := make([]string, 0, len(batch))
		args := make([]interface{}, 0, len(batch)*columns)
		for _, loc := range batch {
			placeholders = append(placeholders, "('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?, ?, ?)")
			args = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.AddedAt, addedOrderByPath[loc.Path], loc.Extension)
		}
		query := `INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, added_at, added_order, extension) VALUES ` +
			strings.Join(placeholders, ",") +
			` ON CONFLICT(path) DO UPDATE SET
				content_hash=excluded.content_hash,
				size_bytes=excluded.size_bytes,
				mod_time=excluded.mod_time,
				extension=excluded.extension`
		if _, err := q.Exec(query, args...); err != nil {
			return err
		}

		managedPlaceholders := make([]string, 0, len(batch))
		managedArgs := make([]interface{}, 0, len(batch)*2)
		for _, loc := range batch {
			if strings.TrimSpace(loc.StoragePath) == "" {
				continue
			}
			managedPlaceholders = append(managedPlaceholders, "(?, ?)")
			managedArgs = append(managedArgs, loc.StoragePath, loc.Path)
		}
		if len(managedPlaceholders) > 0 {
			managedQuery := `WITH managed(physical_path, path) AS (VALUES ` +
				strings.Join(managedPlaceholders, ",") +
				`)
				INSERT INTO managed_storage_locations (location_id, physical_path)
				SELECT l.id, managed.physical_path
				FROM managed
				JOIN locations l ON l.path = managed.path
				WHERE true
				ON CONFLICT(location_id) DO UPDATE SET physical_path = excluded.physical_path`
			if _, err := q.Exec(managedQuery, managedArgs...); err != nil {
				return err
			}
		}
	}
	return nil
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

// GetAllFilesInfoPageSortedAddedOrder preserves the existing sort behavior for
// every sort except added. Added ordering uses the full durable tuple
// (added_at, added_order, id), including in the keyset cursor predicate.
func (s *Store) GetAllFilesInfoPageSortedAddedOrder(limit int, cursor *types.PageCursor, sort string, order string) ([]types.FileInfo, error) {
	if sort != "added" {
		return s.GetAllFilesInfoPageSorted(limit, cursor, sort, order)
	}
	cursorClause, cursorArgs, err := s.addedOrderCursorClause(cursor, order)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT %s
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE 1=1 %s
		ORDER BY %s
		LIMIT ?
	`, fileInfoColumns(), cursorClause, addedOrderSortClause(order))
	args := append(cursorArgs, limit)
	return s.scanFileInfos(query, args...)
}

func (s *Store) GetFilesInfoByLocationQueryPageSortedAddedOrder(query string, args []interface{}, limit int, cursor *types.PageCursor, sort string, order string) ([]types.FileInfo, error) {
	if sort != "added" {
		return s.GetFilesInfoByLocationQueryPageSorted(query, args, limit, cursor, sort, order)
	}
	cursorClause, cursorArgs, err := s.addedOrderCursorClause(cursor, order)
	if err != nil {
		return nil, err
	}
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT %s
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE 1=1 %s
		ORDER BY %s
		LIMIT ?
	`, query, fileInfoColumns(), cursorClause, addedOrderSortClause(order))
	pagedArgs := append(append([]interface{}{}, args...), cursorArgs...)
	pagedArgs = append(pagedArgs, limit)
	return s.scanFileInfos(finalQuery, pagedArgs...)
}

func (s *Store) GetAllFilesInfoPageSortedOffsetAddedOrder(limit int, offset int, sort string, order string) ([]types.FileInfo, error) {
	if sort != "added" {
		return s.GetAllFilesInfoPageSortedOffset(limit, offset, sort, order)
	}
	query := fmt.Sprintf(`
		SELECT %s
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, fileInfoColumns(), addedOrderSortClause(order))
	return s.scanFileInfos(query, limit, offset)
}

func (s *Store) GetFilesInfoByLocationQueryPageSortedOffsetAddedOrder(query string, args []interface{}, limit int, offset int, sort string, order string) ([]types.FileInfo, error) {
	if sort != "added" {
		return s.GetFilesInfoByLocationQueryPageSortedOffset(query, args, limit, offset, sort, order)
	}
	finalQuery := fmt.Sprintf(`
		WITH result_locations(id) AS (%s)
		SELECT %s
		FROM locations l
		JOIN result_locations rl ON l.id = rl.id
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, query, fileInfoColumns(), addedOrderSortClause(order))
	pagedArgs := append(append([]interface{}{}, args...), limit, offset)
	return s.scanFileInfos(finalQuery, pagedArgs...)
}
