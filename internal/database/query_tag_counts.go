package database

import (
	"strings"

	"gooru.local/types"
)

// BatchGetQueryTagCounts returns cardinality estimates keyed by exact query spelling.
// Exact tags use tags.files_count; explicit namespace-only filters such as "artist:"
// use the distinct-file aggregate maintained in tag_key_counts.
func (s *Store) BatchGetQueryTagCounts(tagStrings []string) (map[string]int, error) {
	counts := make(map[string]int)
	if len(tagStrings) == 0 {
		return counts, nil
	}

	exact := make([]types.ParsedTag, 0, len(tagStrings))
	namespaceKeys := make([]string, 0, len(tagStrings))
	seenNamespaces := make(map[string]struct{})
	for _, tagStr := range tagStrings {
		key, value, hasColon := strings.Cut(tagStr, ":")
		if hasColon && value == "" {
			if key != "" {
				if _, seen := seenNamespaces[key]; !seen {
					seenNamespaces[key] = struct{}{}
					namespaceKeys = append(namespaceKeys, key)
				}
			}
			continue
		}
		if !hasColon {
			key = tagStr
		}
		exact = append(exact, types.ParsedTag{Key: key, Value: value})
	}

	exactCounts, err := s.BatchGetTagCounts(exact)
	if err != nil {
		return nil, err
	}
	for tag, count := range exactCounts {
		counts[tag] = count
	}
	if len(namespaceKeys) == 0 {
		return counts, nil
	}

	placeholders := strings.Repeat("?,", len(namespaceKeys)-1) + "?"
	args := make([]interface{}, len(namespaceKeys))
	for i, key := range namespaceKeys {
		args[i] = key
	}
	rows, err := s.Query("SELECT key, files_count FROM tag_key_counts WHERE key IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		counts[key+":"] = count
	}
	return counts, rows.Err()
}
