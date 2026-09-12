package database

import (
	"database/sql"
	"strings"

	"gooru.local/types"
)

// GetFileInfosByPublicIDs resolves tracked files in the caller's requested
// public-ID order. Queries are chunked below SQLite's variable limit and attach
// managed physical storage in the same read, avoiding per-file lookup chains.
func (s *Store) GetFileInfosByPublicIDs(publicIDs []string) ([]types.FileInfo, error) {
	if len(publicIDs) == 0 {
		return nil, nil
	}

	filesByID := make(map[string]types.FileInfo, len(publicIDs))
	for start := 0; start < len(publicIDs); start += maxVars {
		end := start + maxVars
		if end > len(publicIDs) {
			end = len(publicIDs)
		}
		chunk := publicIDs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := `SELECT ` + fileInfoColumns() + `, msl.physical_path
			FROM locations l
			LEFT JOIN media_metadata mm ON mm.location_id = l.id
			LEFT JOIN managed_storage_locations msl ON msl.location_id = l.id
			WHERE l.public_id IN (` + placeholders + `)`
		args := make([]interface{}, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}

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
			filesByID[file.PublicID] = file
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	files := make([]types.FileInfo, 0, len(publicIDs))
	for _, id := range publicIDs {
		file, ok := filesByID[id]
		if !ok {
			return nil, sql.ErrNoRows
		}
		files = append(files, file)
	}
	return files, nil
}
