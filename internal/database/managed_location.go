package database

import (
	"database/sql"
	"fmt"

	"gooru.local/types"
)

// GetLocationSourceByPath returns the tracked logical location together with an
// optional managed physical storage path. Callers that need to read file bytes
// should prefer StoragePath when it is non-empty while continuing to use Path as
// the database identity.
func (s *Store) GetLocationSourceByPath(path string) (types.LocationInfo, error) {
	var loc types.LocationInfo
	var storagePath sql.NullString
	err := s.QueryRow(`
		SELECT l.content_hash, l.size_bytes, l.mod_time, l.extension, l.tags_cache,
		       msl.physical_path
		FROM locations l
		LEFT JOIN managed_storage_locations msl ON msl.location_id = l.id
		WHERE l.path = ?`, path).Scan(
		&loc.Hash, &loc.Size, &loc.ModTime, &loc.Extension, &loc.TagsCache, &storagePath,
	)
	if err != nil {
		return types.LocationInfo{}, err
	}
	loc.Path = path
	if storagePath.Valid {
		loc.StoragePath = storagePath.String
	}
	return loc, nil
}

// BatchGetLocationSourcesByPaths returns tracked logical locations together with
// optional managed physical storage paths in bounded batches. This avoids an
// N+1 lookup when callers need to test many tracked sources at once.
func (s *Store) BatchGetLocationSourcesByPaths(paths []string) (map[string]types.LocationInfo, error) {
	locations := make(map[string]types.LocationInfo)
	if len(paths) == 0 {
		return locations, nil
	}

	for placeholders, args := range stringBindBatches(paths) {
		query := `
			SELECT l.path, l.content_hash, l.size_bytes, l.mod_time, l.extension, l.tags_cache,
			       msl.physical_path
			FROM locations l
			LEFT JOIN managed_storage_locations msl ON msl.location_id = l.id
			WHERE l.path IN (` + placeholders + `)`

		rows, err := s.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var loc types.LocationInfo
			var storagePath sql.NullString
			if err := rows.Scan(
				&loc.Path, &loc.Hash, &loc.Size, &loc.ModTime, &loc.Extension, &loc.TagsCache, &storagePath,
			); err != nil {
				rows.Close()
				return nil, err
			}
			if storagePath.Valid {
				loc.StoragePath = storagePath.String
			}
			locations[loc.Path] = loc
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	return locations, nil
}

// BatchGetManagedLocationSourcesByPhysicalPaths returns managed locations keyed
// by the physical paths supplied by a filesystem scan. Relink uses this to hide
// backing-store aliases that are already represented by canonical logical
// locations, even when those logical paths are outside the current scan scope.
func (s *Store) BatchGetManagedLocationSourcesByPhysicalPaths(paths []string) (map[string]types.LocationInfo, error) {
	locations := make(map[string]types.LocationInfo)
	if len(paths) == 0 {
		return locations, nil
	}

	for placeholders, args := range stringBindBatches(paths) {
		query := `
			SELECT l.path, l.content_hash, l.size_bytes, l.mod_time, l.extension, l.tags_cache,
			       msl.physical_path
			FROM managed_storage_locations msl
			JOIN locations l ON l.id = msl.location_id
			WHERE msl.physical_path IN (` + placeholders + `)`

		rows, err := s.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var loc types.LocationInfo
			if err := rows.Scan(
				&loc.Path, &loc.Hash, &loc.Size, &loc.ModTime, &loc.Extension, &loc.TagsCache, &loc.StoragePath,
			); err != nil {
				rows.Close()
				return nil, err
			}
			locations[loc.StoragePath] = loc
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	return locations, nil
}

// UpdateRelinkedLocation applies a relink move while respecting managed storage
// indirection. For a managed location, the planner's destination is the newly
// discovered physical source, so repair that mapping and preserve the canonical
// logical path and location-scoped metadata. Unmanaged locations keep the
// ordinary path-move behavior.
func (s *Store) UpdateRelinkedLocation(q Querier, oldPath string, newInfo types.LocationInfo) error {
	res, err := q.Exec(`
		UPDATE managed_storage_locations
		SET physical_path = ?
		WHERE location_id = (SELECT id FROM locations WHERE path = ?)`,
		newInfo.Path, oldPath)
	if err != nil {
		return err
	}
	managedRows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if managedRows == 0 {
		return s.UpdateMovedLocation(q, oldPath, newInfo)
	}

	res, err = q.Exec(`
		UPDATE locations
		SET size_bytes = ?, mod_time = ?
		WHERE path = ?`,
		newInfo.Size, newInfo.ModTime, oldPath)
	if err != nil {
		return err
	}
	updatedRows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if updatedRows != 1 {
		return fmt.Errorf("expected to update 1 managed location for %q, updated %d", oldPath, updatedRows)
	}
	return nil
}