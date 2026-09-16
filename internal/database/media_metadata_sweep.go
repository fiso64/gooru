package database

import (
	"fmt"

	"gooru.local/types"
)

// ListPendingMediaMetadataFiles returns a keyset-paginated page containing one
// representative location for each content hash that has not yet had media
// metadata processed. Choosing the lowest live location ID for a hash prevents
// duplicate paths for the same bytes from triggering redundant extraction while
// preserving the durable location-ID cursor used by existing sweep tasks.
func (s *Store) ListPendingMediaMetadataFiles(afterLocationID int64, limit int) ([]types.FileInfo, error) {
	if afterLocationID < 0 {
		return nil, fmt.Errorf("media metadata sweep cursor must not be negative")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("media metadata sweep limit must be positive")
	}
	query := `SELECT ` + fileInfoColumns() + `
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE l.id > ?
		  AND mm.content_hash IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM locations earlier
			WHERE earlier.content_hash = l.content_hash
			  AND earlier.id < l.id
		  )
		ORDER BY l.id
		LIMIT ?`
	return s.scanFileInfos(query, afterLocationID, limit)
}

// UpsertMediaMetadataForLocation persists metadata by content identity only
// while the representative location is still the exact logical file that was
// analysed. Both content hash and path are guarded because provider selection
// is path-dependent. A false result means the snapshot went stale and no row
// was written.
func (s *Store) UpsertMediaMetadataForLocation(locationID int64, expectedHash, expectedPath string, meta types.MediaMetadata) (bool, error) {
	result, err := s.Exec(`
		INSERT INTO media_metadata (
			content_hash, media_kind, mime_type, image_width, image_height,
			video_width, video_height, duration_seconds, frame_count, page_count, updated_at
		)
		SELECT l.content_hash, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP
		FROM locations l
		WHERE l.id = ? AND l.content_hash = ? AND l.path = ?
		ON CONFLICT(content_hash) DO UPDATE SET
			media_kind = excluded.media_kind,
			mime_type = excluded.mime_type,
			image_width = excluded.image_width,
			image_height = excluded.image_height,
			video_width = excluded.video_width,
			video_height = excluded.video_height,
			duration_seconds = excluded.duration_seconds,
			frame_count = excluded.frame_count,
			page_count = excluded.page_count,
			updated_at = CURRENT_TIMESTAMP`,
		meta.MediaKind, meta.MimeType, meta.ImageWidth, meta.ImageHeight,
		meta.VideoWidth, meta.VideoHeight, meta.DurationSeconds, meta.FrameCount,
		meta.PageCount, locationID, expectedHash, expectedPath,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
