package database

import (
	"fmt"
	"strings"

	"gooru.local/types"
)

// TagFacetExclusion describes one simple user-tag exclusion. KeyOnly matches
// every value in the namespace; otherwise Tag is an exact key/value match.
type TagFacetExclusion struct {
	Tag     types.ParsedTag
	KeyOnly bool
}

func scanKindFacets(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
	Close() error
}) ([]types.TagWithCount, error) {
	defer rows.Close()
	var out []types.TagWithCount
	for rows.Next() {
		var item types.TagWithCount
		if err := rows.Scan(&item.Tag, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// KindFacetsForTag reads the transactionally maintained per-tag kind partition.
// Counts are per tracked location, matching KindFacetsByLocationQuery semantics.
func (s *Store) KindFacetsForTag(key, value string) ([]types.TagWithCount, error) {
	rows, err := s.Query(`
		SELECT tk.kind, tk.files_count
		FROM tags t
		JOIN tag_kind_counts tk ON tk.tag_id = t.id
		WHERE t.key = ? AND t.value = ? AND tk.files_count > 0
		ORDER BY tk.files_count DESC, tk.kind ASC
	`, key, value)
	if err != nil {
		return nil, err
	}
	return scanKindFacets(rows)
}

// KindFacetsForTagKey reads the deduplicated key-wide kind partition. A content
// with several values under the same key contributes each tracked location once.
func (s *Store) KindFacetsForTagKey(key string) ([]types.TagWithCount, error) {
	rows, err := s.Query(`
		SELECT kind, files_count
		FROM tag_key_kind_counts
		WHERE key = ? AND files_count > 0
		ORDER BY files_count DESC, kind ASC
	`, key)
	if err != nil {
		return nil, err
	}
	return scanKindFacets(rows)
}

// KindFacetsForExcludedTags returns the kind partition of the union of content
// carrying any exclusion. It starts from indexed tag associations and only
// joins locations for excluded content, so work scales with the hidden subset
// instead of the visible library. DISTINCT preserves union semantics when a
// content item carries more than one excluded tag.
func (s *Store) KindFacetsForExcludedTags(exclusions []TagFacetExclusion) ([]types.TagWithCount, error) {
	if len(exclusions) == 0 {
		return nil, nil
	}

	conditions := make([]string, 0, len(exclusions))
	args := make([]interface{}, 0, len(exclusions)*2)
	for _, exclusion := range exclusions {
		if exclusion.KeyOnly {
			conditions = append(conditions, "t.key = ?")
			args = append(args, exclusion.Tag.Key)
			continue
		}
		conditions = append(conditions, "(t.key = ? AND t.value = ?)")
		args = append(args, exclusion.Tag.Key, exclusion.Tag.Value)
	}

	query := fmt.Sprintf(`
		WITH excluded_contents(content_hash) AS (
			SELECT DISTINCT ct.content_hash
			FROM tags t
			JOIN content_tags ct ON ct.tag_id = t.id
			WHERE %s
		)
		SELECT %s AS kind, COUNT(*) AS files_count
		FROM excluded_contents ec
		JOIN locations l ON l.content_hash = ec.content_hash
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		GROUP BY kind
		ORDER BY files_count DESC, kind ASC
	`, strings.Join(conditions, " OR "), fileKindExpression())

	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanKindFacets(rows)
}
