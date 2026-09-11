package database

import (
	"fmt"
	"strings"
)

// RemoveLocationsByPublicIDTx removes many stable public location identities in
// one caller-owned transaction. SQL is split only to respect SQLite's bound
// variable limit; every chunk remains part of the same transaction.
func (s *Store) RemoveLocationsByPublicIDTx(q Querier, publicIDs []string) (int, error) {
	if len(publicIDs) == 0 {
		return 0, nil
	}

	batchSize := maxVars
	totalRemoved := 0
	for start := 0; start < len(publicIDs); start += batchSize {
		end := start + batchSize
		if end > len(publicIDs) {
			end = len(publicIDs)
		}
		batch := publicIDs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]interface{}, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		result, err := q.Exec("DELETE FROM locations WHERE public_id IN ("+placeholders+")", args...)
		if err != nil {
			return 0, fmt.Errorf("remove locations by public id: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("remove locations by public id rows affected: %w", err)
		}
		totalRemoved += int(affected)
	}
	return totalRemoved, nil
}
