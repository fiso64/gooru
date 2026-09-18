package database

import (
	"database/sql"
	"fmt"
	"strings"

	"gooru.local/types"
)

func stringBatchArgs(values []string) (string, []interface{}) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	args := make([]interface{}, len(values))
	for i, value := range values {
		args[i] = value
	}
	return placeholders, args
}

// getFileInfosByColumn resolves tracked locations for a bounded set of values.
// Column names are intentionally restricted here because the column itself is
// interpolated while all caller data remains parameter-bound.
func (s *Store) getFileInfosByColumn(values []string, column string) ([]types.FileInfo, error) {
	if len(values) == 0 {
		return nil, nil
	}
	switch column {
	case "l.public_id", "l.path", "l.content_hash":
	default:
		return nil, fmt.Errorf("unsupported file-info batch column %q", column)
	}
	orderBy := ""
	if column == "l.content_hash" {
		orderBy = " ORDER BY l.content_hash, l.path, l.id"
	}

	files := make([]types.FileInfo, 0, len(values))
	for start := 0; start < len(values); start += maxVars {
		end := start + maxVars
		if end > len(values) {
			end = len(values)
		}
		placeholders, args := stringBatchArgs(values[start:end])
		query := `SELECT ` + fileInfoColumns() + `, msl.physical_path
			FROM locations l
			LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
			LEFT JOIN managed_storage_locations msl ON msl.location_id = l.id
			WHERE ` + column + ` IN (` + placeholders + `)` + orderBy

		rows, err := s.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var row fileInfoRow
			var storagePath sql.NullString
			if err := rows.Scan(row.scanTargets(&storagePath)...); err != nil {
				rows.Close()
				return nil, err
			}
			file := row.value()
			if storagePath.Valid {
				file.StoragePath = storagePath.String
			}
			files = append(files, file)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return files, nil
}

// GetFileInfosByPublicIDs resolves tracked files in the caller's requested
// public-ID order. Queries are chunked below SQLite's variable limit and attach
// managed physical storage in the same read, avoiding per-file lookup chains.
func (s *Store) GetFileInfosByPublicIDs(publicIDs []string) ([]types.FileInfo, error) {
	files, err := s.getFileInfosByColumn(publicIDs, "l.public_id")
	if err != nil {
		return nil, err
	}
	filesByID := make(map[string]types.FileInfo, len(files))
	for _, file := range files {
		filesByID[file.PublicID] = file
	}

	ordered := make([]types.FileInfo, 0, len(publicIDs))
	for _, id := range publicIDs {
		file, ok := filesByID[id]
		if !ok {
			return nil, sql.ErrNoRows
		}
		ordered = append(ordered, file)
	}
	return ordered, nil
}

// GetFileInfosByPaths resolves only currently tracked paths and returns them by
// exact path. Missing paths are intentionally omitted so callers can attach
// context-specific errors without relying on query result ordering.
func (s *Store) GetFileInfosByPaths(paths []string) (map[string]types.FileInfo, error) {
	files, err := s.getFileInfosByColumn(paths, "l.path")
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]types.FileInfo, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	return byPath, nil
}

// GetFileInfosByContentHashes resolves one deterministic tracked location per
// content hash. Hashes with no current location are omitted so upload duplicate
// detection requires an actually viewable tracked file rather than content
// identity alone.
func (s *Store) GetFileInfosByContentHashes(hashes []string) (map[string]types.FileInfo, error) {
	files, err := s.getFileInfosByColumn(hashes, "l.content_hash")
	if err != nil {
		return nil, err
	}
	byHash := make(map[string]types.FileInfo, len(files))
	for _, file := range files {
		if _, exists := byHash[file.Hash]; !exists {
			byHash[file.Hash] = file
		}
	}
	return byHash, nil
}
