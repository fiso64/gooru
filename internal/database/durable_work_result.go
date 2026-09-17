package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const maxBackgroundOperationResultBytes = 1 << 20

// SetBackgroundOperationResult persists the structured success result for an active
// logical operation. Results are deliberately bounded so a producer cannot turn the
// operation history table into an unbounded blob store. The result is only exposed to
// consumers after the operation reaches completed state.
func (s *Store) SetBackgroundOperationResult(operationID string, resultJSON []byte) error {
	if s == nil || s.DB == nil {
		return errors.New("background operation store is required")
	}
	return s.setBackgroundOperationResult(s.DB, operationID, resultJSON)
}

// SetBackgroundOperationResultTx persists a success payload inside an
// existing domain transaction so consumers can never observe a completed
// mutation whose durable replay result was lost in a separate commit.
func (s *Store) SetBackgroundOperationResultTx(tx *Tx, operationID string, resultJSON []byte) error {
	if s == nil {
		return errors.New("background operation store is required")
	}
	return s.setBackgroundOperationResult(tx, operationID, resultJSON)
}

func (s *Store) setBackgroundOperationResult(q Querier, operationID string, resultJSON []byte) error {
	if q == nil {
		return errors.New("background operation querier is required")
	}
	if operationID == "" {
		return errors.New("background operation id is required")
	}
	if len(resultJSON) == 0 || !json.Valid(resultJSON) {
		return errors.New("background operation result must be valid JSON")
	}
	if len(resultJSON) > maxBackgroundOperationResultBytes {
		return fmt.Errorf("background operation result exceeds %d bytes", maxBackgroundOperationResultBytes)
	}
	summary := summarizeBackgroundOperationResult(resultJSON)
	if isBackgroundTagMutationDurableResult(resultJSON) {
		var affected sql.NullInt64
		if err := q.QueryRow(`
			SELECT affected_count
			FROM background_tag_mutations
			WHERE operation_id = ?
		`, operationID).Scan(&affected); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("background tag mutation result is missing")
			}
			return fmt.Errorf("read background tag mutation result summary: %w", err)
		}
		if !affected.Valid {
			return errors.New("background tag mutation result is missing affected count")
		}
		if affected.Int64 < 0 {
			return errors.New("background tag mutation result affected count cannot be negative")
		}
		value := affected.Int64
		summary.AffectedCount = &value
	}
	res, err := q.Exec(`
		UPDATE background_operations
		SET result_json = ?,
		    result_outcome = ?,
		    result_affected_count = ?,
		    result_failed_count = ?
		WHERE id = ? AND status IN ('pending', 'running')
	`, string(resultJSON), summary.Outcome, nullableBackgroundResultCount(summary.AffectedCount), nullableBackgroundResultCount(summary.FailedCount), operationID)
	if err != nil {
		return fmt.Errorf("set background operation result: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background operation result rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("background operation is missing or no longer active")
	}
	return nil
}

func isBackgroundTagMutationDurableResult(resultJSON []byte) bool {
	var marker struct {
		DurableResult string `json:"durable_result"`
	}
	if err := json.Unmarshal(resultJSON, &marker); err != nil {
		return false
	}
	return marker.DurableResult == "tag_mutation"
}

func summarizeBackgroundOperationResult(resultJSON []byte) BackgroundOperationResultSummary {
	zero := int64(0)
	summary := BackgroundOperationResultSummary{Outcome: "success", FailedCount: &zero}
	var projection struct {
		Files *[]struct {
			Status string `json:"status"`
		} `json:"files"`
		MatchedFiles  *int64 `json:"matched_files"`
		AffectedCount *int64 `json:"affected_count"`
	}
	if err := json.Unmarshal(resultJSON, &projection); err != nil {
		return summary
	}
	if projection.Files != nil {
		var imported, failed int64
		for _, file := range *projection.Files {
			switch file.Status {
			case "imported":
				imported++
			case "error":
				failed++
			}
		}
		summary.AffectedCount = &imported
		summary.FailedCount = &failed
		if failed > 0 {
			if failed == int64(len(*projection.Files)) {
				summary.Outcome = "error"
			} else {
				summary.Outcome = "partial_success"
			}
		}
		return summary
	}
	if projection.MatchedFiles != nil && *projection.MatchedFiles >= 0 {
		summary.AffectedCount = projection.MatchedFiles
		return summary
	}
	if projection.AffectedCount != nil && *projection.AffectedCount >= 0 {
		summary.AffectedCount = projection.AffectedCount
	}
	return summary
}

func nullableBackgroundResultCount(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// GetBackgroundOperationResult returns a completed operation's structured result. An
// active, failed, canceled, missing, or result-less operation returns found=false so
// callers cannot observe a success payload before the lifecycle says it is complete.
func (s *Store) GetBackgroundOperationResult(operationID string) ([]byte, bool, error) {
	if s == nil || s.DB == nil {
		return nil, false, errors.New("background operation store is required")
	}
	if operationID == "" {
		return nil, false, errors.New("background operation id is required")
	}
	var result sql.NullString
	if err := s.DB.QueryRow(`
		SELECT result_json
		FROM background_operations
		WHERE id = ? AND status = 'completed' AND COALESCE(result_json, '') <> ''
	`, operationID).Scan(&result); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read background operation result: %w", err)
	}
	if !result.Valid || result.String == "" {
		return nil, false, nil
	}
	return []byte(result.String), true, nil
}
