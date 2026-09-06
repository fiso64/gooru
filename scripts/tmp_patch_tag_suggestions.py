from pathlib import Path

path = Path("internal/database/database.go")
text = path.read_text()

start = text.index("func (s *Store) ListTagSuggestions(prefix string, limit int)")
end = text.index("func (s *Store) ListNamespaceSuggestions", start)
replacement = r'''func (s *Store) ListTagSuggestions(prefix string, limit int) ([]types.TagWithCount, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	prefix = strings.TrimSpace(prefix)

	var rows *sql.Rows
	var err error
	switch {
	case prefix == "":
		rows, err = s.Query(`
			SELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
			FROM tags
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, limit)
	case strings.ContainsAny(prefix, "%_"):
		// Preserve the historical raw-LIKE wildcard behavior for direct callers.
		// Normal interactive prefixes take the indexed paths below.
		like := strings.ToLower(prefix) + "%"
		rows, err = s.Query(`
			SELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
			FROM tags
			WHERE lower(key) LIKE ? OR lower(key || ':' || value) LIKE ?
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, like, like, limit)
	case strings.Contains(prefix, ":"):
		namespace, valuePrefix, _ := strings.Cut(prefix, ":")
		rows, err = s.Query(`
			SELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
			FROM tags
			WHERE key = ? AND value LIKE ?
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, namespace, valuePrefix+"%", limit)
	default:
		rows, err = s.Query(`
			SELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
			FROM tags
			WHERE key LIKE ?
			ORDER BY files_count DESC, tag_str ASC
			LIMIT ?
		`, prefix+"%", limit)
	}
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

'''
text = text[:start] + replacement + text[end:]

start = text.index("func (s *Store) ListTagValueSuggestions(namespace string, valuePrefix string, limit int)")
end = text.index("func (s *Store) ListTagNamespaces", start)
replacement = r'''func (s *Store) ListTagValueSuggestions(namespace string, valuePrefix string, limit int) ([]types.TagWithCount, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	like := strings.TrimSpace(valuePrefix) + "%"
	rows, err := s.Query(`
		SELECT key || ':' || value AS tag_str, files_count
		FROM tags
		WHERE value != '' AND key = ? AND value LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?
	`, strings.TrimSpace(namespace), like, limit)
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

'''
text = text[:start] + replacement + text[end:]
path.write_text(text)
