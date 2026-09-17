package database

// GetKnownSizes returns the distinct logical file sizes currently tracked by the store.
func (s *Store) GetKnownSizes() (map[int64]struct{}, error) {
	rows, err := s.Query("SELECT DISTINCT size_bytes FROM locations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[int64]struct{})
	for rows.Next() {
		var size int64
		if err := rows.Scan(&size); err != nil {
			return nil, err
		}
		sizes[size] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sizes, nil
}
