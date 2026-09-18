package database

import (
	"database/sql"
	"fmt"
)

// BackgroundOperationResultSummary is the small, list-safe projection of a
// structured operation result. The full result remains in result_json for
// explicit detail/recovery reads.
type BackgroundOperationResultSummary struct {
	Outcome       string
	AffectedCount *int64
	FailedCount   *int64
}

// GetBackgroundOperationResultSummaries loads compact summaries for a bounded
// set of operations without materializing result_json. Missing summaries are
// omitted so callers can fall back to lifecycle-only presentation for rows
// created before summary persistence existed.
func (s *Store) GetBackgroundOperationResultSummaries(operationIDs []string) (map[string]BackgroundOperationResultSummary, error) {
	summaries := make(map[string]BackgroundOperationResultSummary)
	for start := 0; start < len(operationIDs); start += maxVars {
		end := min(start+maxVars, len(operationIDs))
		placeholders, args := stringBatchArgs(operationIDs[start:end])
		rows, err := s.Query(`
			SELECT id, result_outcome, result_affected_count, result_failed_count
			FROM background_operations
			WHERE id IN (`+placeholders+`) AND COALESCE(result_outcome, '') <> ''
		`, args...)
		if err != nil {
			return nil, fmt.Errorf("read background operation result summaries: %w", err)
		}
		for rows.Next() {
			var operationID string
			var outcome sql.NullString
			var affected, failed sql.NullInt64
			if err := rows.Scan(&operationID, &outcome, &affected, &failed); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan background operation result summary: %w", err)
			}
			summary := BackgroundOperationResultSummary{Outcome: outcome.String}
			if affected.Valid {
				value := affected.Int64
				summary.AffectedCount = &value
			}
			if failed.Valid {
				value := failed.Int64
				summary.FailedCount = &value
			}
			summaries[operationID] = summary
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate background operation result summaries: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close background operation result summaries: %w", err)
		}
	}
	return summaries, nil
}
