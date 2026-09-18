package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	BackgroundTagTargetFileID      = "file_id"
	BackgroundTagTargetContentHash = "content_hash"
)

type BackgroundTagMutation struct {
	OperationID       string
	Mutation          string
	SelectorJSON      []byte
	TagsJSON          []byte
	TargetKind        string
	MatchedFiles      int
	AffectedCount     int
	NotificationsJSON []byte
	ResultReady       bool
}

func (s *Store) CreateBackgroundTagMutationSnapshot(
	operationID string,
	mutation string,
	selectorJSON []byte,
	tagsJSON []byte,
	targetKind string,
	targetIDs []string,
	targetQuery string,
	targetArgs []interface{},
) (int, error) {
	if s == nil || s.DB == nil {
		return 0, errors.New("background tag mutation store is required")
	}
	tx, err := s.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin background tag mutation snapshot: %w", err)
	}
	defer tx.Rollback()

	matched, err := s.createBackgroundTagMutationSnapshot(
		tx,
		operationID,
		mutation,
		selectorJSON,
		tagsJSON,
		targetKind,
		targetIDs,
		targetQuery,
		targetArgs,
	)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit background tag mutation snapshot: %w", err)
	}
	return matched, nil
}

func (s *Store) createBackgroundTagMutationSnapshot(
	q Querier,
	operationID string,
	mutation string,
	selectorJSON []byte,
	tagsJSON []byte,
	targetKind string,
	targetIDs []string,
	targetQuery string,
	targetArgs []interface{},
) (int, error) {
	if q == nil {
		return 0, errors.New("background tag mutation querier is required")
	}
	if operationID == "" {
		return 0, errors.New("background tag mutation operation id is required")
	}
	if mutation != "add" && mutation != "set" && mutation != "remove" {
		return 0, fmt.Errorf("invalid background tag mutation %q", mutation)
	}
	if !json.Valid(selectorJSON) {
		return 0, errors.New("background tag mutation selector must be valid JSON")
	}
	if !json.Valid(tagsJSON) {
		return 0, errors.New("background tag mutation tags must be valid JSON")
	}
	if targetKind != BackgroundTagTargetFileID && targetKind != BackgroundTagTargetContentHash {
		return 0, fmt.Errorf("invalid background tag mutation target kind %q", targetKind)
	}
	if targetKind == BackgroundTagTargetFileID && targetQuery != "" {
		return 0, errors.New("file-id background tag mutation cannot use a target query")
	}
	if targetKind == BackgroundTagTargetContentHash && len(targetIDs) != 0 {
		return 0, errors.New("content-hash background tag mutation cannot use explicit targets")
	}

	var visible int
	var status string
	var attached int
	if err := q.QueryRow(`
		SELECT visible, status,
		       (SELECT count(*) FROM background_tasks WHERE operation_id = background_operations.id)
		FROM background_operations
		WHERE id = ?
	`, operationID).Scan(&visible, &status, &attached); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("background tag mutation operation is missing")
		}
		return 0, fmt.Errorf("inspect background tag mutation reservation: %w", err)
	}
	if visible != 0 || status != string(BackgroundWorkPending) || attached != 0 {
		return 0, errors.New("background tag mutation operation is not an unattached hidden pending reservation")
	}

	if _, err := q.Exec(`
		INSERT INTO background_tag_mutations
			(operation_id, mutation, selector_json, tags_json, target_kind, matched_files)
		VALUES (?, ?, ?, ?, ?, 0)
	`, operationID, mutation, string(selectorJSON), string(tagsJSON), targetKind); err != nil {
		return 0, fmt.Errorf("create background tag mutation: %w", err)
	}

	matched := 0
	switch targetKind {
	case BackgroundTagTargetFileID:
		const columns = 3
		batchSize := maxVars / columns
		for start := 0; start < len(targetIDs); start += batchSize {
			end := start + batchSize
			if end > len(targetIDs) {
				end = len(targetIDs)
			}
			var query strings.Builder
			query.WriteString(`INSERT OR IGNORE INTO background_tag_mutation_targets
				(operation_id, target_id, target_order)
			VALUES `)
			args := make([]interface{}, 0, (end-start)*columns)
			for index := start; index < end; index++ {
				targetID := targetIDs[index]
				if targetID == "" {
					return 0, fmt.Errorf("background tag mutation target %d is blank", index)
				}
				if index > start {
					query.WriteString(", ")
				}
				query.WriteString("(?, ?, ?)")
				args = append(args, operationID, targetID, index)
			}
			result, err := q.Exec(query.String(), args...)
			if err != nil {
				return 0, fmt.Errorf("insert background tag mutation targets %d-%d: %w", start, end-1, err)
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return 0, fmt.Errorf("count inserted background tag mutation targets %d-%d: %w", start, end-1, err)
			}
			matched += int(inserted)
		}
	case BackgroundTagTargetContentHash:
		if targetQuery != "" {
			snapshotQuery := `
				INSERT OR IGNORE INTO background_tag_mutation_targets
					(operation_id, target_id, target_order)
				SELECT ?, hash, 0 FROM (` + targetQuery + `)
			`
			args := make([]interface{}, 0, len(targetArgs)+1)
			args = append(args, operationID)
			args = append(args, targetArgs...)
			result, err := q.Exec(snapshotQuery, args...)
			if err != nil {
				return 0, fmt.Errorf("snapshot background tag mutation query: %w", err)
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return 0, fmt.Errorf("count snapshotted background tag mutation targets: %w", err)
			}
			matched += int(inserted)
		}
	}

	if _, err := q.Exec(`
		UPDATE background_tag_mutations
		SET matched_files = ?
		WHERE operation_id = ?
	`, matched, operationID); err != nil {
		return 0, fmt.Errorf("update background tag mutation target count: %w", err)
	}
	return matched, nil
}

func (s *Store) GetBackgroundTagMutation(operationID string) (BackgroundTagMutation, bool, error) {
	if s == nil || s.DB == nil {
		return BackgroundTagMutation{}, false, errors.New("background tag mutation store is required")
	}
	if operationID == "" {
		return BackgroundTagMutation{}, false, errors.New("background tag mutation operation id is required")
	}
	var mutation BackgroundTagMutation
	var selector string
	var tags string
	var affected sql.NullInt64
	var notifications sql.NullString
	if err := s.DB.QueryRow(`
		SELECT operation_id, mutation, selector_json, tags_json, target_kind,
		       matched_files, affected_count, notifications_json
		FROM background_tag_mutations
		WHERE operation_id = ?
	`, operationID).Scan(
		&mutation.OperationID,
		&mutation.Mutation,
		&selector,
		&tags,
		&mutation.TargetKind,
		&mutation.MatchedFiles,
		&affected,
		&notifications,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BackgroundTagMutation{}, false, nil
		}
		return BackgroundTagMutation{}, false, fmt.Errorf("read background tag mutation: %w", err)
	}
	mutation.SelectorJSON = []byte(selector)
	mutation.TagsJSON = []byte(tags)
	if affected.Valid {
		mutation.AffectedCount = int(affected.Int64)
		mutation.ResultReady = true
	}
	if notifications.Valid {
		mutation.NotificationsJSON = []byte(notifications.String)
	}
	return mutation, true, nil
}

func (s *Store) ListBackgroundTagMutationTargets(operationID string) ([]string, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("background tag mutation store is required")
	}
	rows, err := s.DB.Query(`
		SELECT target_id
		FROM background_tag_mutation_targets
		WHERE operation_id = ?
		ORDER BY target_order ASC, target_id ASC
	`, operationID)
	if err != nil {
		return nil, fmt.Errorf("list background tag mutation targets: %w", err)
	}
	defer rows.Close()
	var targets []string
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, fmt.Errorf("scan background tag mutation target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate background tag mutation targets: %w", err)
	}
	return targets, nil
}

func (s *Store) SetBackgroundTagMutationResultTx(tx *Tx, operationID string, affectedCount int, notificationsJSON []byte) error {
	if s == nil || tx == nil {
		return errors.New("background tag mutation transaction is required")
	}
	if operationID == "" {
		return errors.New("background tag mutation operation id is required")
	}
	if affectedCount < 0 {
		return errors.New("background tag mutation affected count cannot be negative")
	}
	if !json.Valid(notificationsJSON) {
		return errors.New("background tag mutation notifications must be valid JSON")
	}
	res, err := tx.Exec(`
		UPDATE background_tag_mutations
		SET affected_count = ?, notifications_json = ?
		WHERE operation_id = ? AND affected_count IS NULL
	`, affectedCount, string(notificationsJSON), operationID)
	if err != nil {
		return fmt.Errorf("set background tag mutation result: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background tag mutation result rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("background tag mutation result is missing or already finalized")
	}
	return nil
}
