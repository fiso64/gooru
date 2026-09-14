package database

import (
	"database/sql"
	"fmt"
	"strings"

	"gooru.local/types"
)

// ListPendingMediaMetadataContentHashes returns a bounded set of distinct
// content identities for which at least one currently tracked location has not
// yet had media metadata processed.
func (s *Store) ListPendingMediaMetadataContentHashes(limit int) ([]string, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("media metadata sweep limit must be positive")
	}
	rows, err := s.Query(`
		SELECT l.content_hash
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.location_id = l.id
		WHERE mm.location_id IS NULL
		GROUP BY l.content_hash
		ORDER BY MIN(l.id)
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		if strings.TrimSpace(hash) != "" {
			hashes = append(hashes, hash)
		}
	}
	return hashes, rows.Err()
}

// GetMediaMetadataByContentHash returns any cached metadata for a currently
// tracked sibling location with the same immutable content identity.
func (s *Store) GetMediaMetadataByContentHash(hash string) (types.MediaMetadata, bool, error) {
	var meta types.MediaMetadata
	err := s.QueryRow(`
		SELECT mm.location_id, mm.media_kind, mm.mime_type,
		       mm.image_width, mm.image_height, mm.video_width, mm.video_height,
		       mm.duration_seconds, mm.frame_count, mm.page_count
		FROM media_metadata mm
		JOIN locations l ON l.id = mm.location_id
		WHERE l.content_hash = ?
		ORDER BY l.id
		LIMIT 1`, hash).Scan(
		&meta.LocationID, &meta.MediaKind, &meta.MimeType,
		&meta.ImageWidth, &meta.ImageHeight, &meta.VideoWidth, &meta.VideoHeight,
		&meta.DurationSeconds, &meta.FrameCount, &meta.PageCount,
	)
	if err == sql.ErrNoRows {
		return types.MediaMetadata{}, false, nil
	}
	if err != nil {
		return types.MediaMetadata{}, false, err
	}
	return meta, true, nil
}

// UpsertMediaMetadataForContentHash fans one extraction result out only to
// locations that still point at hash when the write occurs. This prevents a
// stale worker from attaching metadata to content that has since been replaced.
func (s *Store) UpsertMediaMetadataForContentHash(hash string, meta types.MediaMetadata) error {
	_, err := s.Exec(`
		INSERT INTO media_metadata (
			location_id, media_kind, mime_type, image_width, image_height,
			video_width, video_height, duration_seconds, frame_count, page_count, updated_at
		)
		SELECT l.id, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP
		FROM locations l
		WHERE l.content_hash = ?
		ON CONFLICT(location_id) DO UPDATE SET
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
		meta.PageCount, hash,
	)
	return err
}
