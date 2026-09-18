package database

import (
	"strings"

	"gooru.local/types"
)

// BatchGetQueryTagCounts returns cardinality estimates keyed by exact query spelling.
// Bare key-wide tags and explicit namespace-only filters use the distinct-file
// aggregate maintained in tag_key_counts; exact value tags use tags.files_count.
func (s *Store) BatchGetQueryTagCounts(tagStrings []string) (map[string]int, error) {
	counts := make(map[string]int, len(tagStrings))
	if len(tagStrings) == 0 {
		return counts, nil
	}

	exact := make([]types.ParsedTag, 0, len(tagStrings))
	keySpellings := make(map[string][]string)
	keys := make([]string, 0, len(tagStrings))
	seenKeys := make(map[string]struct{})
	for _, tagStr := range tagStrings {
		// Keep an explicit zero for absent tags. Query planning may distinguish a
		// known-empty tag from one whose cardinality was not available.
		counts[tagStr] = 0
		key, value, hasColon := strings.Cut(tagStr, ":")
		if !hasColon {
			key = tagStr
		}
		if (!hasColon || value == "") && key != "" {
			keySpellings[key] = append(keySpellings[key], tagStr)
			if _, seen := seenKeys[key]; !seen {
				seenKeys[key] = struct{}{}
				keys = append(keys, key)
			}
			continue
		}
		exact = append(exact, types.ParsedTag{Key: key, Value: value})
	}

	const exactColumns = 2 // key, value
	exactBatchSize := maxVars / exactColumns
	for start := 0; start < len(exact); start += exactBatchSize {
		end := start + exactBatchSize
		if end > len(exact) {
			end = len(exact)
		}
		exactCounts, err := s.BatchGetTagCounts(exact[start:end])
		if err != nil {
			return nil, err
		}
		for tag, count := range exactCounts {
			counts[tag] = count
		}
	}
	if len(keys) == 0 {
		return counts, nil
	}

	for placeholders, args := range stringBindBatches(keys) {
		rows, err := s.Query("SELECT key, files_count FROM tag_key_counts WHERE key IN ("+placeholders+")", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key string
			var count int
			if err := rows.Scan(&key, &count); err != nil {
				rows.Close()
				return nil, err
			}
			for _, spelling := range keySpellings[key] {
				counts[spelling] = count
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return counts, nil
}
