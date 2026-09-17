package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FindActiveAssociatedBackgroundOperationID returns the active auxiliary
// operation associated with one producer and auxiliary kind. Both operations
// must still be active; callers may create and bind a replacement when a prior
// auxiliary was explicitly canceled.
func (s *Store) FindActiveAssociatedBackgroundOperationID(q Querier, producerOperationID, auxiliaryKind string) (string, bool, error) {
	if q == nil {
		return "", false, errors.New("background operation querier is required")
	}
	if producerOperationID == "" {
		return "", false, errors.New("background producer operation id is required")
	}
	if auxiliaryKind == "" {
		return "", false, errors.New("background auxiliary operation kind is required")
	}

	var auxiliaryOperationID string
	err := q.QueryRow(`
		SELECT association.auxiliary_operation_id
		FROM background_operation_associations AS association
		JOIN background_operations AS producer
		  ON producer.id = association.producer_operation_id
		JOIN background_operations AS auxiliary
		  ON auxiliary.id = association.auxiliary_operation_id
		WHERE association.producer_operation_id = ?
		  AND association.auxiliary_kind = ?
		  AND producer.status IN ('pending', 'running')
		  AND auxiliary.status IN ('pending', 'running')
		LIMIT 1
	`, producerOperationID, auxiliaryKind).Scan(&auxiliaryOperationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find associated background operation: %w", err)
	}
	return auxiliaryOperationID, true, nil
}

// AssociateBackgroundOperation durably binds one auxiliary operation kind to a
// producer without making the auxiliary a progress child of the producer. A
// terminal prior auxiliary may be replaced while the producer remains active;
// an active association is never stolen.
func (s *Store) AssociateBackgroundOperation(q Querier, producerOperationID, auxiliaryKind, auxiliaryOperationID string, createdAt time.Time) error {
	if q == nil {
		return errors.New("background operation querier is required")
	}
	if producerOperationID == "" {
		return errors.New("background producer operation id is required")
	}
	if auxiliaryKind == "" {
		return errors.New("background auxiliary operation kind is required")
	}
	if auxiliaryOperationID == "" {
		return errors.New("background auxiliary operation id is required")
	}
	if producerOperationID == auxiliaryOperationID {
		return errors.New("background producer and auxiliary operation ids must differ")
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	createdAt = normalizeWorkTime(createdAt)

	result, err := q.Exec(`
		INSERT INTO background_operation_associations (
			producer_operation_id, auxiliary_kind, auxiliary_operation_id, created_at
		)
		SELECT ?, ?, ?, ?
		WHERE EXISTS (
			SELECT 1
			FROM background_operations
			WHERE id = ? AND status IN ('pending', 'running')
		)
		AND EXISTS (
			SELECT 1
			FROM background_operations
			WHERE id = ? AND kind = ? AND status IN ('pending', 'running')
		)
		ON CONFLICT(producer_operation_id, auxiliary_kind) DO UPDATE SET
			auxiliary_operation_id = excluded.auxiliary_operation_id,
			created_at = excluded.created_at
		WHERE EXISTS (
			SELECT 1
			FROM background_operations AS previous_auxiliary
			WHERE previous_auxiliary.id = background_operation_associations.auxiliary_operation_id
			  AND previous_auxiliary.status NOT IN ('pending', 'running')
		)
	`, producerOperationID, auxiliaryKind, auxiliaryOperationID, workTimeValue(createdAt), producerOperationID, auxiliaryOperationID, auxiliaryKind)
	if err != nil {
		return fmt.Errorf("associate background operation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("associate background operation rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("associate background operation: producer is not active, auxiliary is invalid, or an active association already exists")
	}
	return nil
}
