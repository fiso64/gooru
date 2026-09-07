package database

import (
	"fmt"
	"strings"

	"gooru.local/types"
)

// TagFacetExclusion describes one user-tag selector whose matching content is
// excluded from a kind facet. KeyOnly selectors match every value in the tag
// namespace; exact selectors match one key/value pair.
type TagFacetExclusion struct {
	Key     string
	Value   string
	KeyOnly bool
}

// KindFacetsForTagExclusions returns the effective-kind counts for the union of
// content matched by the supplied tag exclusions. Each selector is driven from
// the indexed tags/content_tags side and UNION deduplicates content matched by
// more than one exclusion before locations are expanded, avoiding double
// subtraction when callers remove these counts from the maintained root facet.
//
// This is intentionally used for multiple exclusions. Single exclusions can be
// answered entirely from tag_kind_counts/tag_key_kind_counts without visiting
// content associations at all.
func (s *Store) KindFacetsForTagExclusions(exclusions []TagFacetExclusion) ([]types.TagWithCount, error) {
	if len(exclusions) == 0 {
		return nil, nil
	}

	branches := make([]string, 0, len(exclusions))
	args := make([]interface{}, 0, len(exclusions)*2)
	for _, exclusion := range exclusions {
		if exclusion.KeyOnly {
			branches = append(branches, `
				SELECT ct.content_hash
				FROM tags t
				JOIN content_tags ct ON ct.tag_id = t.id
				WHERE t.key = ?`)
			args = append(args, exclusion.Key)
			continue
		}
		branches = append(branches, `
			SELECT ct.content_hash
			FROM tags t
			JOIN content_tags ct ON ct.tag_id = t.id
			WHERE t.key = ? AND t.value = ?`)
		args = append(args, exclusion.Key, exclusion.Value)
	}

	query := fmt.Sprintf(`
		WITH excluded_content AS (
			%s
		)
		SELECT CASE
			WHEN lower(l.extension) = '.cbz' THEN 'comic'
			ELSE coalesce(mm.media_kind, CASE
				WHEN lower(l.extension) = '.gif' THEN 'gif'
				WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
				WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
				ELSE 'other'
			END)
		END AS kind,
		COUNT(*) AS files_count
		FROM excluded_content ec
		JOIN locations l ON l.content_hash = ec.content_hash
		LEFT JOIN media_metadata mm ON mm.location_id = l.id
		GROUP BY kind
		ORDER BY files_count DESC, kind ASC
	`, strings.Join(branches, "\nUNION\n"))

	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	facets := make([]types.TagWithCount, 0, 4)
	for rows.Next() {
		var facet types.TagWithCount
		if err := rows.Scan(&facet.Tag, &facet.Count); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}
	return facets, rows.Err()
}
