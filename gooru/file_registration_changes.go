package gooru

import (
	"fmt"
	"strings"

	"gooru.local/types"
)

const fileRegistrationLocationLookupBatchSize = 400

// fileRegistrationHashesForLocationUpserts reports content identities whose
// tracked location will actually be inserted or changed by the current
// transaction. A tag-only mutation of an already-tracked path is not a
// registration event, while a new path for an already-known content hash is.
//
// The lookup is bounded by the candidate paths supplied by the caller; it never
// scans the library, which keeps external scans and large imports scalable.
func fileRegistrationHashesForLocationUpserts(q databaseQuerier, locations map[string]types.LocationInfo) ([]string, error) {
	if q == nil {
		return nil, fmt.Errorf("file registration transaction querier is required")
	}
	if len(locations) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(locations))
	for path := range locations {
		paths = append(paths, path)
	}
	existingHashes := make(map[string]string, len(locations))
	for start := 0; start < len(paths); start += fileRegistrationLocationLookupBatchSize {
		end := start + fileRegistrationLocationLookupBatchSize
		if end > len(paths) {
			end = len(paths)
		}
		batch := paths[start:end]
		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch))
		for i, path := range batch {
			placeholders[i] = "?"
			args[i] = path
		}
		rows, err := q.Query(`SELECT path, content_hash FROM locations WHERE path IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("inspect existing registration locations: %w", err)
		}
		for rows.Next() {
			var path, hash string
			if err := rows.Scan(&path, &hash); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("read existing registration location: %w", err)
			}
			existingHashes[path] = hash
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate existing registration locations: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close existing registration locations: %w", err)
		}
	}

	changed := make([]string, 0, len(locations))
	for path, location := range locations {
		if existingHash, ok := existingHashes[path]; !ok || existingHash != location.Hash {
			changed = append(changed, location.Hash)
		}
	}
	return uniqueRegistrationHashes(changed), nil
}

// fileRegistrationBackgroundTasksForChangedLocations builds hook work from a
// location-change signal that has already been classified inside the surrounding
// transaction. The core default therefore does not need to guess registration
// from content-hash existence, which would miss duplicate-content locations.
func (c *Client) fileRegistrationBackgroundTasksForChangedLocations(hashes []string, operationID string) ([]BackgroundTaskRequest, error) {
	unique := uniqueRegistrationHashes(hashes)
	if len(unique) == 0 {
		return nil, nil
	}
	if c.fileRegistrationHooks == nil {
		return mediaMetadataRegistrationTasksForOperation(true, strings.TrimSpace(operationID))
	}
	return c.fileRegistrationBackgroundTasksForOperation(unique, operationID)
}
