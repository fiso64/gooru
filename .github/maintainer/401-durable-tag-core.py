#!/usr/bin/env python3
from pathlib import Path

def write(path: str, content: str) -> None:
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")

def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one replacement target, found {count}")
    p.write_text(text.replace(old, new, 1), encoding="utf-8")

write("internal/database/migrations/028_background_tag_mutations.up.sql", r'''CREATE TABLE background_tag_mutations (
    operation_id TEXT PRIMARY KEY,
    mutation TEXT NOT NULL CHECK (mutation IN ('add', 'set', 'remove')),
    selector_json TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('file_id', 'content_hash')),
    matched_files INTEGER NOT NULL DEFAULT 0 CHECK (matched_files >= 0),
    affected_count INTEGER,
    notifications_json TEXT,
    FOREIGN KEY (operation_id) REFERENCES background_operations(id) ON DELETE CASCADE
);

CREATE TABLE background_tag_mutation_targets (
    operation_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    target_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (operation_id, target_id),
    FOREIGN KEY (operation_id) REFERENCES background_tag_mutations(operation_id) ON DELETE CASCADE
);

CREATE INDEX idx_background_tag_mutation_targets_order
ON background_tag_mutation_targets(operation_id, target_order, target_id);
''')

write("internal/database/migrations/028_background_tag_mutations.down.sql", r'''DROP TABLE IF EXISTS background_tag_mutation_targets;
DROP TABLE IF EXISTS background_tag_mutations;
''')

write("internal/database/durable_tag_mutation.go", r'''package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

	tx, err := s.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin background tag mutation snapshot: %w", err)
	}
	defer tx.Rollback()

	var visible int
	var status string
	var attached int
	if err := tx.QueryRow(`
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

	if _, err := tx.Exec(`
		INSERT INTO background_tag_mutations
			(operation_id, mutation, selector_json, tags_json, target_kind, matched_files)
		VALUES (?, ?, ?, ?, ?, 0)
	`, operationID, mutation, string(selectorJSON), string(tagsJSON), targetKind); err != nil {
		return 0, fmt.Errorf("create background tag mutation: %w", err)
	}

	switch targetKind {
	case BackgroundTagTargetFileID:
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO background_tag_mutation_targets
				(operation_id, target_id, target_order)
			VALUES (?, ?, ?)
		`)
		if err != nil {
			return 0, fmt.Errorf("prepare background tag mutation targets: %w", err)
		}
		defer stmt.Close()
		for index, targetID := range targetIDs {
			if targetID == "" {
				return 0, fmt.Errorf("background tag mutation target %d is blank", index)
			}
			if _, err := stmt.Exec(operationID, targetID, index); err != nil {
				return 0, fmt.Errorf("insert background tag mutation target %d: %w", index, err)
			}
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
			if _, err := tx.Exec(snapshotQuery, args...); err != nil {
				return 0, fmt.Errorf("snapshot background tag mutation query: %w", err)
			}
		}
	}

	var matched int
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM background_tag_mutation_targets
		WHERE operation_id = ?
	`, operationID).Scan(&matched); err != nil {
		return 0, fmt.Errorf("count background tag mutation targets: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE background_tag_mutations
		SET matched_files = ?
		WHERE operation_id = ?
	`, matched, operationID); err != nil {
		return 0, fmt.Errorf("update background tag mutation target count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit background tag mutation snapshot: %w", err)
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
''')

write("gooru/background_tag_mutation.go", r'''package gooru

import (
	"encoding/json"
	"errors"
	"fmt"

	"gooru.local/internal/query"
	"gooru.local/types"
)

const (
	BackgroundTagMutationOperationKind = "tag_mutation"
	BackgroundTagMutationTaskKind      = "metadata.tag_mutation"
	BackgroundTagMutationResourceClass = "metadata"

	backgroundTagMutationTargetFileID      = "file_id"
	backgroundTagMutationTargetContentHash = "content_hash"
	backgroundTagMutationInputVersion      = 1
)

type BackgroundTagMutationRequest struct {
	Mutation       string
	Selector       any
	Tags           []string
	FileIDs        []string
	Query          string
	ExcludedHashes []string
	MaxPending     int
}

type BackgroundTagMutationState struct {
	OperationID   string
	Mutation      string
	SelectorJSON  json.RawMessage
	Tags          []string
	TargetKind    string
	MatchedFiles  int
	AffectedCount int
	Notifications []types.Notification
	ResultReady   bool
}

func (c *Client) CreateBackgroundTagMutation(request BackgroundTagMutationRequest) (BackgroundOperation, error) {
	if _, err := backgroundTagMutationKind(request.Mutation, request.Tags); err != nil {
		return BackgroundOperation{}, err
	}
	if request.MaxPending < 1 {
		return BackgroundOperation{}, errors.New("background tag mutation pending limit must be positive")
	}
	if (len(request.FileIDs) == 0) == (request.Query == "") {
		return BackgroundOperation{}, errors.New("background tag mutation requires exactly one target selector")
	}
	if request.Query == "" && len(request.ExcludedHashes) != 0 {
		return BackgroundOperation{}, errors.New("background tag mutation file-id selector cannot use excluded hashes")
	}
	if request.Mutation != "remove" || len(request.Tags) > 0 {
		if err := query.ValidateTags(request.Tags); err != nil {
			return BackgroundOperation{}, err
		}
	}
	selectorJSON, err := json.Marshal(request.Selector)
	if err != nil {
		return BackgroundOperation{}, fmt.Errorf("encode background tag mutation selector: %w", err)
	}
	tagsJSON, err := json.Marshal(request.Tags)
	if err != nil {
		return BackgroundOperation{}, fmt.Errorf("encode background tag mutation tags: %w", err)
	}

	operation, err := c.CreateBackgroundOperationWithPendingLimit(BackgroundOperationRequest{
		Kind:          BackgroundTagMutationOperationKind,
		Visible:       false,
		ProgressTotal: 1,
	}, request.MaxPending)
	if err != nil {
		return BackgroundOperation{}, err
	}
	attached := false
	defer func() {
		if !attached {
			_, _ = c.CancelBackgroundOperation(operation.ID)
		}
	}()

	targetKind := backgroundTagMutationTargetFileID
	var targetQuery string
	var targetArgs []interface{}
	if request.Query != "" {
		targetKind = backgroundTagMutationTargetContentHash
		targetQuery, targetArgs, err = c.buildQuery(request.Query)
		if err != nil {
			return BackgroundOperation{}, err
		}
		if targetQuery != "" {
			targetQuery, targetArgs = excludeContentHashes(targetQuery, targetArgs, request.ExcludedHashes)
		}
	}
	if _, err := c.store.CreateBackgroundTagMutationSnapshot(
		operation.ID,
		request.Mutation,
		selectorJSON,
		tagsJSON,
		targetKind,
		request.FileIDs,
		targetQuery,
		targetArgs,
	); err != nil {
		return BackgroundOperation{}, err
	}

	if _, err := c.AttachBackgroundTaskAndRevealOperation(
		operation.ID,
		map[string]int{"version": backgroundTagMutationInputVersion},
		BackgroundTaskRequest{
			DedupeKey:     "mutate",
			Kind:          BackgroundTagMutationTaskKind,
			SubjectKind:   "operation",
			SubjectID:     operation.ID,
			InputKey:      "v1",
			ResourceClass: BackgroundTagMutationResourceClass,
			MaxAttempts:   5,
		},
	); err != nil {
		return BackgroundOperation{}, err
	}
	attached = true
	operation.Visible = true
	return operation, nil
}

func (c *Client) GetBackgroundTagMutation(operationID string) (BackgroundTagMutationState, bool, error) {
	stored, found, err := c.store.GetBackgroundTagMutation(operationID)
	if err != nil || !found {
		return BackgroundTagMutationState{}, found, err
	}
	var tags []string
	if err := json.Unmarshal(stored.TagsJSON, &tags); err != nil {
		return BackgroundTagMutationState{}, false, fmt.Errorf("decode background tag mutation tags: %w", err)
	}
	state := BackgroundTagMutationState{
		OperationID:   stored.OperationID,
		Mutation:      stored.Mutation,
		SelectorJSON:  append(json.RawMessage(nil), stored.SelectorJSON...),
		Tags:          tags,
		TargetKind:    stored.TargetKind,
		MatchedFiles:  stored.MatchedFiles,
		AffectedCount: stored.AffectedCount,
		ResultReady:   stored.ResultReady,
	}
	if stored.ResultReady {
		if len(stored.NotificationsJSON) == 0 {
			return BackgroundTagMutationState{}, false, errors.New("background tag mutation result is missing notifications")
		}
		if err := json.Unmarshal(stored.NotificationsJSON, &state.Notifications); err != nil {
			return BackgroundTagMutationState{}, false, fmt.Errorf("decode background tag mutation notifications: %w", err)
		}
	}
	return state, true, nil
}

func (c *Client) ListBackgroundTagMutationTargets(operationID string) ([]string, error) {
	return c.store.ListBackgroundTagMutationTargets(operationID)
}

func (c *Client) ExecuteBackgroundTagMutationQuery(operationID string) error {
	state, found, err := c.GetBackgroundTagMutation(operationID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("background tag mutation is missing")
	}
	if state.ResultReady {
		return nil
	}
	if state.TargetKind != backgroundTagMutationTargetContentHash {
		return fmt.Errorf("background tag mutation %q does not use content-hash targets", operationID)
	}
	kind, err := backgroundTagMutationKind(state.Mutation, state.Tags)
	if err != nil {
		return err
	}

	tx, err := c.store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	sqlQuery := `SELECT target_id AS hash FROM background_tag_mutation_targets WHERE operation_id = ?`
	args := []interface{}{operationID}
	affectedCount, err := c.applyBackgroundTagMutationQueryInTx(tx, sqlQuery, args, state.Tags, kind, state.MatchedFiles)
	if err != nil {
		return err
	}
	result := types.TagOperationResult{AffectedCount: affectedCount}
	if err := c.persistBackgroundTagMutationResultTx(tx, operationID, result); err != nil {
		return err
	}
	return tx.Commit()
}

func (c *Client) ExecuteBackgroundTagMutationPaths(operationID string, paths []string) error {
	state, found, err := c.GetBackgroundTagMutation(operationID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("background tag mutation is missing")
	}
	if state.ResultReady {
		return nil
	}
	if state.TargetKind != backgroundTagMutationTargetFileID {
		return fmt.Errorf("background tag mutation %q does not use file-id targets", operationID)
	}
	if len(paths) != state.MatchedFiles {
		return fmt.Errorf("background tag mutation target count changed: expected %d paths, got %d", state.MatchedFiles, len(paths))
	}
	kind, err := backgroundTagMutationKind(state.Mutation, state.Tags)
	if err != nil {
		return err
	}
	analysis, err := c.analyzeFileStates(paths, nil, false)
	if err != nil {
		return err
	}
	if len(analysis.allFileData) != len(paths) {
		return errors.New("background tag mutation could not analyze every snapshotted path")
	}
	_, _, err = c.executeTaggingTransaction(analysis, state.Tags, kind, nil, nil, func(tx *databaseTx, affectedCount int64, movesHandled map[string]string) error {
		result := backgroundTagOperationResult(analysis, int(affectedCount), movesHandled)
		return c.persistBackgroundTagMutationResultTx(tx, operationID, result)
	})
	return err
}

func (c *Client) persistBackgroundTagMutationResultTx(tx *databaseTx, operationID string, result types.TagOperationResult) error {
	notificationsJSON, err := json.Marshal(result.Notifications)
	if err != nil {
		return fmt.Errorf("encode background tag mutation notifications: %w", err)
	}
	if err := c.store.SetBackgroundTagMutationResultTx(tx, operationID, result.AffectedCount, notificationsJSON); err != nil {
		return err
	}
	if err := c.store.SetBackgroundOperationResultTx(tx, operationID, []byte(`{"durable_result":"tag_mutation"}`)); err != nil {
		return fmt.Errorf("persist background tag mutation operation result marker: %w", err)
	}
	return nil
}

func (c *Client) applyBackgroundTagMutationQueryInTx(
	tx *databaseTx,
	sqlQuery string,
	args []interface{},
	tags []string,
	kind opKind,
	matchedFiles int,
) (int, error) {
	switch kind {
	case opSetTags:
		if _, err := tx.Exec(`DELETE FROM content_tags WHERE content_hash IN (`+sqlQuery+`)`, args...); err != nil {
			return 0, fmt.Errorf("clear snapshotted tags: %w", err)
		}
		if len(tags) > 0 {
			tagIDs, err := c.tagIDsForMutation(tx, tags, true)
			if err != nil {
				return 0, err
			}
			for _, tagID := range tagIDs {
				insertArgs := make([]interface{}, 0, len(args)+1)
				insertArgs = append(insertArgs, tagID)
				insertArgs = append(insertArgs, args...)
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO content_tags (content_hash, tag_id) SELECT hash, ? FROM (`+sqlQuery+`)`,
					insertArgs...,
				); err != nil {
					return 0, fmt.Errorf("associate snapshotted replacement tags: %w", err)
				}
			}
		}
		return matchedFiles, nil
	case opTag:
		if len(tags) == 0 {
			return 0, nil
		}
		tagIDs, err := c.tagIDsForMutation(tx, tags, true)
		if err != nil {
			return 0, err
		}
		affected, err := c.store.BatchAssociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
		return int(affected), err
	case opUntag:
		if len(tags) == 0 {
			if _, err := c.store.BatchClearTagsByContentQueryTx(tx, sqlQuery, args); err != nil {
				return 0, err
			}
			return matchedFiles, nil
		}
		tagIDs, err := c.tagIDsForMutation(tx, tags, false)
		if err != nil {
			return 0, err
		}
		if len(tagIDs) == 0 {
			return 0, nil
		}
		affected, err := c.store.BatchDisassociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
		return int(affected), err
	default:
		return 0, fmt.Errorf("unsupported background tag operation %d", kind)
	}
}

func backgroundTagMutationKind(mutation string, tags []string) (opKind, error) {
	switch mutation {
	case "add":
		return opTag, nil
	case "set":
		return opSetTags, nil
	case "remove":
		if len(tags) == 0 {
			return opSetTags, nil
		}
		return opUntag, nil
	default:
		return 0, fmt.Errorf("unsupported background tag mutation %q", mutation)
	}
}

func backgroundTagOperationResult(analysis *fileStateAnalysis, affectedCount int, movesHandled map[string]string) types.TagOperationResult {
	result := types.TagOperationResult{AffectedCount: affectedCount}
	for _, data := range analysis.allFileData {
		if data.wasModified {
			result.Notifications = append(result.Notifications, types.Notification{
				Kind:         types.NotificationKindModified,
				OriginalPath: data.path,
				OrphanedTags: data.orphanedTags,
			})
		} else if oldPath, ok := movesHandled[data.info.Path]; ok {
			result.Notifications = append(result.Notifications, types.Notification{
				Kind:         types.NotificationKindMoveDetected,
				OriginalPath: data.path,
				OldPath:      oldPath,
				NewPath:      data.path,
			})
		}
	}
	return result
}
''')

write("gooru/background_tag_mutation_test.go", r'''package gooru

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func newBackgroundTagMutationTestClient(t *testing.T) *Client {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init database: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func writeBackgroundTagMutationTestFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestBackgroundTagMutationQueryUsesAdmissionSnapshot(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	first := writeBackgroundTagMutationTestFile(t, dir, "first.jpg", "first")
	if _, err := client.TagFiles([]string{first}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("tag first file: %v", err)
	}

	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       []string{"reviewed"},
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create background mutation: %v", err)
	}

	second := writeBackgroundTagMutationTestFile(t, dir, "second.jpg", "second")
	if _, err := client.TagFiles([]string{second}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("tag second file: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("execute snapshotted query: %v", err)
	}

	firstTags, _, err := client.GetTagsForFile(first, false)
	if err != nil {
		t.Fatalf("read first tags: %v", err)
	}
	secondTags, _, err := client.GetTagsForFile(second, false)
	if err != nil {
		t.Fatalf("read second tags: %v", err)
	}
	if !containsBackgroundTag(firstTags, "reviewed") {
		t.Fatalf("snapshotted file did not receive reviewed tag: %v", firstTags)
	}
	if containsBackgroundTag(secondTags, "reviewed") {
		t.Fatalf("post-admission query match was mutated: %v", secondTags)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read background mutation: found=%v err=%v", found, err)
	}
	if state.MatchedFiles != 1 || state.AffectedCount != 1 || !state.ResultReady {
		t.Fatalf("unexpected result state: %+v", state)
	}
}

func TestBackgroundTagMutationReplayReturnsStoredResult(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       []string{"reviewed"},
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("first execution: %v", err)
	}
	firstState, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read first result: found=%v err=%v", found, err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("replay execution: %v", err)
	}
	secondState, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read replay result: found=%v err=%v", found, err)
	}
	firstJSON, _ := json.Marshal(firstState)
	secondJSON, _ := json.Marshal(secondState)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("replay changed exact result:\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
}

func TestBackgroundTagMutationCancellationRollsBackTagWrite(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       []string{"reviewed"},
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	canceled, err := client.CancelBackgroundOperation(operation.ID)
	if err != nil || !canceled {
		t.Fatalf("cancel mutation: canceled=%v err=%v", canceled, err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err == nil {
		t.Fatal("expected canceled operation execution to fail")
	}
	tags, _, err := client.GetTagsForFile(path, false)
	if err != nil {
		t.Fatalf("read tags after cancellation: %v", err)
	}
	if containsBackgroundTag(tags, "reviewed") {
		t.Fatalf("canceled mutation committed domain write: %v", tags)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read canceled mutation: found=%v err=%v", found, err)
	}
	if state.ResultReady {
		t.Fatalf("canceled mutation published result: %+v", state)
	}
}

func TestBackgroundTagMutationPathResultPersistsMoveNotification(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	oldPath := writeBackgroundTagMutationTestFile(t, dir, "old.jpg", "body")
	if _, err := client.TagFiles([]string{oldPath}, []string{"initial"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil || len(files) != 1 {
		t.Fatalf("list files: files=%v err=%v", files, err)
	}
	publicID := client.PublicFileID(files[0].ID)
	if publicID == "" {
		t.Fatal("missing public file id")
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string][]string{"file_ids": []string{publicID}},
		Tags:       []string{"reviewed"},
		FileIDs:    []string{publicID},
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	newPath := filepath.Join(dir, "new.jpg")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationPaths(operation.ID, []string{newPath}); err != nil {
		t.Fatalf("execute path mutation: %v", err)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read mutation result: found=%v err=%v", found, err)
	}
	if !state.ResultReady || state.AffectedCount != 1 || len(state.Notifications) != 1 {
		t.Fatalf("unexpected move result: %+v", state)
	}
	notification := state.Notifications[0]
	if notification.Kind != types.NotificationKindMoveDetected || notification.OldPath != oldPath || notification.NewPath != newPath {
		t.Fatalf("unexpected move notification: %+v", notification)
	}
}

func containsBackgroundTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}
''')

replace_once(
    "gooru/tagging.go",
    "type BackgroundOperationTransactionStateBuilder func(affectedCount int) (BackgroundOperationTransactionState, error)\n",
    "type BackgroundOperationTransactionStateBuilder func(affectedCount int) (BackgroundOperationTransactionState, error)\n\ntype taggingTransactionFinalizer func(tx *databaseTx, affectedCount int64, movesHandled map[string]string) error\n",
)

replace_once(
    "gooru/tagging.go",
    "affectedCount, _, err := c.executeTaggingTransaction(analysis, tags, opTag, tasks, stateBuilder)",
    "affectedCount, _, err := c.executeTaggingTransaction(analysis, tags, opTag, tasks, stateBuilder, nil)",
)

replace_once(
    "gooru/tagging.go",
    "func (c *Client) executeTaggingTransaction(analysis *fileStateAnalysis, tags []string, kind opKind, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder) (int64, map[string]string, error) {",
    "func (c *Client) executeTaggingTransaction(analysis *fileStateAnalysis, tags []string, kind opKind, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, finalizer taggingTransactionFinalizer) (int64, map[string]string, error) {",
)

needle = '''\t\tif err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {
\t\t\treturn 0, nil, fmt.Errorf("persist background operation transaction state: %w", err)
\t\t}
\t}

\treturn affectedCount, movesHandled, tx.Commit()
'''
replacement = '''\t\tif err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {
\t\t\treturn 0, nil, fmt.Errorf("persist background operation transaction state: %w", err)
\t\t}
\t}

\tif finalizer != nil {
\t\tif err := finalizer(tx, affectedCount, movesHandled); err != nil {
\t\t\treturn 0, nil, fmt.Errorf("finalize tagging transaction: %w", err)
\t\t}
\t}

\treturn affectedCount, movesHandled, tx.Commit()
'''
replace_once("gooru/tagging.go", needle, replacement)

replace_once(
    "gooru/tagging.go",
    "affectedCount, movesHandled, err := c.executeTaggingTransaction(analysis, tags, kind, nil, nil)",
    "affectedCount, movesHandled, err := c.executeTaggingTransaction(analysis, tags, kind, nil, nil, nil)",
)
