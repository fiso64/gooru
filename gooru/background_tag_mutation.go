package gooru

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

	BackgroundTagMutationTargetFileID      = "file_id"
	BackgroundTagMutationTargetContentHash = "content_hash"
	backgroundTagMutationInputVersion      = 1
)

type BackgroundTagMutationRequest struct {
	Mutation       string
	Selector       any
	Tags           []string
	FileIDs        []string
	Query          string
	FileIDSelector bool
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
	if request.FileIDSelector {
		if request.Query != "" {
			return BackgroundOperation{}, errors.New("background tag mutation file-id selector cannot use a query")
		}
	} else if request.Query == "" || len(request.FileIDs) != 0 {
		return BackgroundOperation{}, errors.New("background tag mutation requires exactly one target selector")
	}
	if request.FileIDSelector && len(request.ExcludedHashes) != 0 {
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

	targetKind := BackgroundTagMutationTargetFileID
	var targetQuery string
	var targetArgs []interface{}
	if !request.FileIDSelector {
		targetKind = BackgroundTagMutationTargetContentHash
		targetQuery, targetArgs, err = c.buildQuery(request.Query)
		if err != nil {
			return BackgroundOperation{}, err
		}
		if targetQuery != "" {
			targetQuery, targetArgs = excludeContentHashes(targetQuery, targetArgs, request.ExcludedHashes)
		}
	}

	operationID, err := newBackgroundWorkID("operation")
	if err != nil {
		return BackgroundOperation{}, err
	}
	taskID, err := newBackgroundWorkID("task")
	if err != nil {
		return BackgroundOperation{}, err
	}
	taskRequest, err := bindBackgroundChildTask(BackgroundTaskRequest{
		DedupeKey:     "mutate",
		Kind:          BackgroundTagMutationTaskKind,
		SubjectKind:   "operation",
		SubjectID:     operationID,
		InputKey:      "v1",
		ResourceClass: BackgroundTagMutationResourceClass,
		MaxAttempts:   5,
	}, operationID, 0)
	if err != nil {
		return BackgroundOperation{}, err
	}
	checkpointJSON, err := json.Marshal(map[string]int{"version": backgroundTagMutationInputVersion})
	if err != nil {
		return BackgroundOperation{}, fmt.Errorf("encode background tag mutation checkpoint: %w", err)
	}
	operation, created, err := createDatabaseBackgroundTagMutationWithTask(
		c,
		operationID,
		taskID,
		BackgroundOperationRequest{
			Kind:          BackgroundTagMutationOperationKind,
			Visible:       false,
			ProgressTotal: 1,
		},
		request.MaxPending,
		request.Mutation,
		selectorJSON,
		tagsJSON,
		targetKind,
		request.FileIDs,
		targetQuery,
		targetArgs,
		checkpointJSON,
		taskRequest,
	)
	if err != nil {
		return BackgroundOperation{}, err
	}
	if !created {
		return BackgroundOperation{}, ErrBackgroundOperationPendingLimit
	}
	c.notifyBackgroundOperationChange()
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
	return c.executeBackgroundTagMutationQuery(BackgroundTask{OperationID: operationID})
}

func (c *Client) ExecuteClaimedBackgroundTagMutationQuery(task BackgroundTask) error {
	if task.ID == "" || task.OperationID == "" || task.claimAttempt < 1 {
		return errors.New("claimed background tag mutation task generation is required")
	}
	return c.executeBackgroundTagMutationQuery(task)
}

func (c *Client) executeBackgroundTagMutationQuery(task BackgroundTask) error {
	operationID := task.OperationID
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
	if state.TargetKind != BackgroundTagMutationTargetContentHash {
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
	if task.ID != "" {
		if err := completeDatabaseBackgroundTaskAttemptTx(c, tx, task); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	c.notifyBackgroundOperationChange()
	if task.ID != "" {
		return backgroundTaskFinalizedByHandlerError()
	}
	return nil
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
	if state.TargetKind != BackgroundTagMutationTargetFileID {
		return fmt.Errorf("background tag mutation %q does not use file-id targets", operationID)
	}
	if len(paths) != state.MatchedFiles {
		return fmt.Errorf("background tag mutation target count changed: expected %d paths, got %d", state.MatchedFiles, len(paths))
	}
	if len(paths) == 0 {
		tx, err := c.store.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := c.persistBackgroundTagMutationResultTx(tx, operationID, types.TagOperationResult{}); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		c.notifyBackgroundOperationChange()
		return nil
	}
	kind, err := backgroundTagMutationKind(state.Mutation, state.Tags)
	if err != nil {
		return err
	}
	analysis, err := c.analyzeFileStates(paths, nil, true)
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
	if err == nil {
		c.notifyBackgroundOperationChange()
	}
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
			if _, err := c.store.BatchAssociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs); err != nil {
				return 0, fmt.Errorf("associate snapshotted replacement tags: %w", err)
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
