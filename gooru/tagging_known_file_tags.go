package gooru

import (
	"encoding/json"
	"errors"
	"fmt"

	"gooru.local/internal/database"
	"gooru.local/internal/query"
	"gooru.local/types"
)

// TagKnownFilesWithBackgroundTasksByFileTags atomically registers known files
// and applies the aligned tag set for each file while enqueuing durable follow-up
// work in the same transaction.
func (c *Client) TagKnownFilesWithBackgroundTasksByFileTags(files []types.LocationInfo, fileTags [][]string, tasks []BackgroundTaskRequest, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	return c.TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState(files, fileTags, tasks, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState extends the
// per-file known-file path with producer-owned operation/task state persisted in
// the same transaction as content, tags, and child tasks.
func (c *Client) TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState(files []types.LocationInfo, fileTags [][]string, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	result := types.TagOperationResult{}
	if len(fileTags) != len(files) {
		return result, fmt.Errorf("per-file tag count %d does not match file count %d", len(fileTags), len(files))
	}

	hashes := make([]string, 0, len(files))
	locations := make(map[string]types.LocationInfo, len(files))
	tagsByHash := make(map[string][]string, len(files))
	validPaths := make([]string, 0, len(files))
	for index, file := range files {
		if err := query.ValidateTags(fileTags[index]); err != nil {
			return result, fmt.Errorf("validate tags for file %d: %w", index, err)
		}
		if file.Path == "" || file.Hash == "" {
			if progressCb != nil {
				progressCb(file.Path, fmt.Errorf("file path and hash are required"))
			}
			continue
		}
		hashes = append(hashes, file.Hash)
		locations[file.Path] = file
		validPaths = append(validPaths, file.Path)
		tagsByHash[file.Hash] = appendUniqueKnownFileTags(tagsByHash[file.Hash], fileTags[index])
	}
	if len(hashes) == 0 {
		return result, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	if err := c.store.BatchInsertContents(tx, hashes); err != nil {
		return result, fmt.Errorf("failed to batch insert contents: %w", err)
	}
	if err := c.store.BatchUpsertLocations(tx, locations); err != nil {
		return result, fmt.Errorf("failed to batch upsert locations: %w", err)
	}
	affectedCount, err := c.associateKnownFileTagsByHash(tx, tagsByHash)
	if err != nil {
		return result, err
	}
	for _, task := range tasks {
		if _, _, err := c.enqueueBackgroundTask(tx, task); err != nil {
			return result, fmt.Errorf("failed to enqueue background task: %w", err)
		}
	}
	if stateBuilder != nil {
		state, err := stateBuilder(int(affectedCount))
		if err != nil {
			return result, fmt.Errorf("build background operation transaction state: %w", err)
		}
		if state.OperationID == "" && state.TaskID == "" {
			return result, errors.New("background transaction state requires an operation or task id")
		}
		checkpointJSON, err := json.Marshal(state.Checkpoint)
		if err != nil {
			return result, fmt.Errorf("encode background transaction checkpoint: %w", err)
		}
		resultJSON, err := json.Marshal(state.Result)
		if err != nil {
			return result, fmt.Errorf("encode background transaction result: %w", err)
		}
		if state.TaskID != "" {
			if err := setDatabaseBackgroundTaskState(c, tx, state.TaskID, checkpointJSON, resultJSON); err != nil {
				return result, fmt.Errorf("persist background task transaction state: %w", err)
			}
		}
		if state.OperationID != "" {
			if err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {
				return result, fmt.Errorf("persist background operation transaction state: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.AffectedCount = int(affectedCount)
	if progressCb != nil {
		for _, path := range validPaths {
			progressCb(path, nil)
		}
	}
	return result, nil
}

func appendUniqueKnownFileTags(existing, additions []string) []string {
	if len(additions) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, tag := range existing {
		seen[tag] = struct{}{}
	}
	for _, tag := range additions {
		if _, ok := seen[tag]; ok {
			continue
		}
		existing = append(existing, tag)
		seen[tag] = struct{}{}
	}
	return existing
}

func (c *Client) associateKnownFileTagsByHash(tx *database.Tx, tagsByHash map[string][]string) (int64, error) {
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
	pairs := make([]database.ContentTagPair, 0, pairCount)
	for hash, tags := range tagsByHash {
		for _, tag := range tags {
			pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagIDMap[tag]})
		}
	}
	affected, err := c.store.BatchAssociateTags(tx, pairs)
	if err != nil {
		return 0, fmt.Errorf("failed to batch associate tags: %w", err)
	}
	return affected, nil
}
