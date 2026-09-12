package database

import (
	"database/sql"

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
