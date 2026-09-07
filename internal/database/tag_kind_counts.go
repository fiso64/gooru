package database

import "gooru.local/types"

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
