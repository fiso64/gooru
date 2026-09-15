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

	files := make([]types.FileInfo, 0, len(values))
	for start := 0; start < len(values); start += maxVars {
		end := start + maxVars
		if end > len(values) {
			end = len(values)
		}
		placeholders, args := stringBatchArgs(values[start:end])
		query := `SELECT ` + fileInfoColumns() + `, msl.physical_path
			FROM locations l
			LEFT JOIN media_metadata mm ON mm.location_id = l.id
			LEFT JOIN managed_storage_locations msl ON msl.location_id = l.id
			WHERE ` + column + ` IN (` + placeholders + `)
			ORDER BY l.content_hash, l.path, l.id`

		rows, err := s.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var file types.FileInfo
			var tagsCache string
			var mediaKind, mimeType, storagePath sql.NullString
			var imageWidth, imageHeight, videoWidth, videoHeight, frameCount, pageCount sql.NullInt64
			var duration sql.NullFloat64
			if err := rows.Scan(
				&file.ID, &file.PublicID, &file.Path, &file.Hash, &file.Size, &file.ModTime, &file.AddedAt, &tagsCache,
				&mediaKind, &mimeType, &imageWidth, &imageHeight, &videoWidth, &videoHeight, &duration, &frameCount, &pageCount,
				&storagePath,
			); err != nil {
				rows.Close()
				return nil, err
			}
			file.Tags = splitTags(tagsCache)
			if storagePath.Valid {
				file.StoragePath = storagePath.String
			}
			if mediaKind.Valid || mimeType.Valid {
				file.Metadata = &types.MediaMetadata{
					LocationID:      file.ID,
					MediaKind:       mediaKind.String,
					MimeType:        mimeType.String,
					ImageWidth:      nullIntPtr(imageWidth),
					ImageHeight:     nullIntPtr(imageHeight),
					VideoWidth:      nullIntPtr(videoWidth),
					VideoHeight:     nullIntPtr(videoHeight),
					DurationSeconds: nullFloatPtr(duration),
					FrameCount:      nullIntPtr(frameCount),
					PageCount:       nullIntPtr(pageCount),
				}
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
// content hash. Content with no current location is omitted; callers that are
// deciding whether an upload is a usable duplicate therefore do not confuse a
// retained content/tag record with an actually viewable file.
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
