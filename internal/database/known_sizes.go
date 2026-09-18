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

// GetHashesBySizes returns the distinct tracked content hashes for the requested logical sizes.
func (s *Store) GetHashesBySizes(sizes []int64) (map[int64]map[string]struct{}, error) {
	hashesBySize := make(map[int64]map[string]struct{})
	for placeholders, args := range bindBatches(sizes) {
		rows, err := s.Query(
			"SELECT DISTINCT size_bytes, content_hash FROM locations WHERE size_bytes IN ("+placeholders+")",
			args...,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var size int64
			var hash string
			if err := rows.Scan(&size, &hash); err != nil {
				rows.Close()
				return nil, err
			}
			hashes := hashesBySize[size]
			if hashes == nil {
				hashes = make(map[string]struct{})
				hashesBySize[size] = hashes
			}
			hashes[hash] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return hashesBySize, nil
}
