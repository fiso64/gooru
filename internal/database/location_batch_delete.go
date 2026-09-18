package database

import "fmt"

// RemoveLocationsByPublicIDTx removes many stable public location identities in
// one caller-owned transaction. SQL is split only to respect SQLite's bound
// variable limit; every chunk remains part of the same transaction.
func (s *Store) RemoveLocationsByPublicIDTx(q Querier, publicIDs []string) (int, error) {
	if len(publicIDs) == 0 {
		return 0, nil
	}

	totalRemoved := 0
	for start := 0; start < len(publicIDs); start += maxVars {
		end := min(start+maxVars, len(publicIDs))
		placeholders, args := stringBatchArgs(publicIDs[start:end])
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
