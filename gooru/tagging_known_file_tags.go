package gooru

import (
	"database/sql"
	"fmt"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// TagKnownFilesWithBackgroundTasksByHashTags registers the supplied known files
// and applies tag sets by content hash in one transaction. The tag map may also
// contain already-tracked hashes, which lets callers update duplicate content
// atomically with newly registered files.
func (c *Client) TagKnownFilesWithBackgroundTasksByHashTags(files []types.LocationInfo, tagsByHash map[string][]string, tasks []BackgroundTaskRequest, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	return c.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(files, tagsByHash, tasks, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState also persists
// producer recovery state in that same transaction.
func (c *Client) TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(files []types.LocationInfo, tagsByHash map[string][]string, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	result := types.TagOperationResult{}
	for hash, tags := range tagsByHash {
		if hash == "" {
			return result, fmt.Errorf("content hash is empty")
		}
		if err := query.ValidateTags(tags); err != nil {
			return result, fmt.Errorf("validate tags for content %q: %w", hash, err)
		}
	}

	hashes := make([]string, 0, len(files))
	locations := make(map[string]types.LocationInfo, len(files))
	validPaths := make([]string, 0, len(files))
	for _, file := range files {
		if file.Path == "" || file.Hash == "" {
			if progressCb != nil {
				progressCb(file.Path, fmt.Errorf("file path and hash are required"))
			}
			continue
		}
		hashes = append(hashes, file.Hash)
		locations[file.Path] = file
		validPaths = append(validPaths, file.Path)
	}
	if len(hashes) == 0 && len(tagsByHash) == 0 {
		return result, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	registrationHashes := []string(nil)
	if len(hashes) > 0 {
		registrationHashes, err = fileRegistrationHashesForLocationUpserts(tx, locations)
		if err != nil {
			return result, fmt.Errorf("classify file registration changes: %w", err)
		}
		if err := c.store.BatchInsertContents(tx, hashes); err != nil {
			return result, fmt.Errorf("failed to batch insert contents: %w", err)
		}
		if err := c.store.BatchUpsertLocations(tx, locations); err != nil {
			return result, fmt.Errorf("failed to batch upsert locations: %w", err)
		}
	}
	affectedCount, err := c.associateKnownFileTagsByHash(tx, tagsByHash)
	if err != nil {
		return result, err
	}

	// Producer-owned operations (notably a browser upload spanning many chunk
	// transactions) are part of the registration event. Build their checkpoint
	// once here so hook-created work can inherit the same lifetime, then reuse the
	// exact state when persisting the producer checkpoint below. Task-scoped state
	// intentionally omits OperationID so a segment cannot overwrite its aggregate
	// operation result; recover that task's parent solely for child-work binding.
	operationID := ""
	effectiveStateBuilder := stateBuilder
	if stateBuilder != nil {
		state, err := stateBuilder(int(affectedCount))
		if err != nil {
			return result, fmt.Errorf("build background operation transaction state: %w", err)
		}
		operationID = strings.TrimSpace(state.OperationID)
		if operationID == "" && strings.TrimSpace(state.TaskID) != "" {
			operationID, err = backgroundTaskOperationIDInTx(tx, state.TaskID)
			if err != nil {
				return result, fmt.Errorf("resolve background task parent operation: %w", err)
			}
		}
		effectiveStateBuilder = func(int) (BackgroundOperationTransactionState, error) {
			return state, nil
		}
	}

	hookTasks, err := c.fileRegistrationBackgroundTasksForChangedLocations(registrationHashes, operationID)
	if err != nil {
		return result, fmt.Errorf("build file registration background tasks: %w", err)
	}
	tasks = append(tasks, hookTasks...)
	operationChanged, err := c.persistTaggingFollowUpTrackedInTx(tx, tasks, effectiveStateBuilder, affectedCount)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	if operationChanged {
		c.notifyBackgroundOperationChange()
	}
	result.AffectedCount = int(affectedCount)
	if progressCb != nil {
		for _, path := range validPaths {
			progressCb(path, nil)
		}
	}
	return result, nil
}

func backgroundTaskOperationIDInTx(tx *databaseTx, taskID string) (string, error) {
	var operationID sql.NullString
	if err := tx.QueryRow(`SELECT operation_id FROM background_tasks WHERE id = ?`, strings.TrimSpace(taskID)).Scan(&operationID); err != nil {
		return "", err
	}
	return strings.TrimSpace(operationID.String), nil
}

func (c *Client) associateKnownFileTagsByHash(tx *databaseTx, tagsByHash map[string][]string) (int64, error) {
	allTags := make([]string, 0)
	seen := make(map[string]struct{})
	pairCount := 0
	for _, tags := range tagsByHash {
		pairCount += len(tags)
		for _, tag := range tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			allTags = append(allTags, tag)
		}
	}
	if len(allTags) == 0 {
		return 0, nil
	}
	parsedTags := make([]types.ParsedTag, len(allTags))
	for index, tag := range allTags {
		parsedTags[index] = query.ParseTag(tag)
	}
	tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
	if err != nil {
		return 0, fmt.Errorf("failed to get or create tags: %w", err)
	}
	pairs := make([]contentTagPair, 0, pairCount)
	for hash, tags := range tagsByHash {
		for _, tag := range tags {
			pairs = append(pairs, contentTagPair{ContentHash: hash, TagID: tagIDMap[tag]})
		}
	}
	affected, err := c.store.BatchAssociateTags(tx, pairs)
	if err != nil {
		return 0, fmt.Errorf("failed to batch associate tags: %w", err)
	}
	return affected, nil
}
