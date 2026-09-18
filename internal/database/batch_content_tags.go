package database

import (
	"fmt"

	"gooru.local/types"
)

// BatchGetTagsForContents returns tags for the requested content hashes while
// bounding each query to SQLite's variable limit. Duplicate hashes are queried
// only once. Hashes with no tags are omitted from the result map.
func (s *Store) BatchGetTagsForContents(hashes []string) (map[string][]string, error) {
	result := make(map[string][]string)
	if len(hashes) == 0 {
		return result, nil
	}

	uniqueHashes := make([]string, 0, len(hashes))
	seen := make(map[string]struct{}, len(hashes))
	for _, hash := range hashes {
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		uniqueHashes = append(uniqueHashes, hash)
	}

	for start := 0; start < len(uniqueHashes); start += maxVars {
		end := min(start+maxVars, len(uniqueHashes))
		placeholders, args := stringBatchArgs(uniqueHashes[start:end])

		rows, err := s.Query(`
			SELECT ct.content_hash, t.key, t.value
			FROM content_tags ct
			JOIN tags t ON t.id = ct.tag_id
			WHERE ct.content_hash IN (`+placeholders+`)
			ORDER BY ct.content_hash, t.key, t.value`, args...)
		if err != nil {
			return result, fmt.Errorf("failed to batch get tags for content: %w", err)
		}

		for rows.Next() {
			var hash string
			var tag types.ParsedTag
			if err := rows.Scan(&hash, &tag.Key, &tag.Value); err != nil {
				rows.Close()
				return result, fmt.Errorf("failed to scan content tags: %w", err)
			}
			result[hash] = append(result[hash], parsedTagString(tag))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return result, fmt.Errorf("failed to iterate content tags: %w", err)
		}
		if err := rows.Close(); err != nil {
			return result, fmt.Errorf("failed to close content tag rows: %w", err)
		}
	}

	return result, nil
}
