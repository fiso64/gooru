package database

import (
	"fmt"

	"gooru.local/types"
)

// KindFacetsForIncludedTagExcludingTags returns the kind partition for content
// that matches included and at least one exclusion. The excluded union is the
// driving set, so an absent hidden tag proves the intersection empty without
// scanning the included library. DISTINCT preserves union semantics when one
// content item carries multiple matching exclusion tags.
func (s *Store) KindFacetsForIncludedTagExcludingTags(included TagFacetExclusion, exclusions []TagFacetExclusion) ([]types.TagWithCount, error) {
	if len(exclusions) == 0 {
		return nil, nil
	}

	exclusionConditions, args := tagFacetConditions("et", exclusions...)
	includeCondition, includeArgs := tagFacetConditions("it", included)
	args = append(args, includeArgs...)

	query := fmt.Sprintf(`
		WITH excluded_contents(content_hash) AS (
			SELECT DISTINCT ect.content_hash
			FROM tags et
			JOIN content_tags ect ON ect.tag_id = et.id
			WHERE %s
		), included_excluded(content_hash) AS (
			SELECT DISTINCT ec.content_hash
			FROM excluded_contents ec
			JOIN content_tags ict ON ict.content_hash = ec.content_hash
			JOIN tags it ON it.id = ict.tag_id
			WHERE %s
		)
		SELECT %s AS kind, COUNT(*) AS files_count
		FROM included_excluded ie
		JOIN locations l ON l.content_hash = ie.content_hash
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		GROUP BY kind
		ORDER BY files_count DESC, kind ASC
	`, exclusionConditions, includeCondition, fileKindExpression())

	rows, err := s.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanKindFacets(rows)
}
