package gooru

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gooru.local/internal/database"
	"gooru.local/internal/query"
	"gooru.local/types"
)

type opKind int

const (
	opTag opKind = iota
	opSetTags
	opUntag
)

// TagFiles adds tags to multiple files using a high-performance batching strategy.
// Returns the number of new tag associations created and a list of notifications.
func (c *Client) TagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadataHeuristic bool) (types.TagOperationResult, error) {
	if err := query.ValidateTags(tags); err != nil {
		return types.TagOperationResult{}, err
	}
	return c.performTagOperation(filePaths, tags, progressCb, opTag, useMetadataHeuristic)
}

// TagKnownFiles imports files whose content hash and filesystem metadata have
// already been computed by the caller, avoiding a second hashing pass.
func (c *Client) TagKnownFiles(files []types.LocationInfo, tags []string, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	return c.TagKnownFilesWithBackgroundTasks(files, tags, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasks atomically registers known files and enqueues
// durable follow-up work. If any task cannot be persisted, file registration and
// tag mutations roll back with it.
type BackgroundOperationTransactionState struct {
	OperationID string
	TaskID      string
	Checkpoint  any
	Result      any
}

type BackgroundOperationTransactionStateBuilder func(affectedCount int) (BackgroundOperationTransactionState, error)

type taggingTransactionFinalizer func(tx *databaseTx, affectedCount int64, movesHandled map[string]string) error

func (c *Client) TagKnownFilesWithBackgroundTasks(files []types.LocationInfo, tags []string, tasks []BackgroundTaskRequest, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	return c.TagKnownFilesWithBackgroundTasksAndOperationState(files, tags, tasks, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasksAndOperationState extends known-file
// registration with producer-owned operation and/or task checkpoint/result
// updates that commit in the same transaction as content, tags, and child tasks.
func (c *Client) TagKnownFilesWithBackgroundTasksAndOperationState(files []types.LocationInfo, tags []string, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
	result := types.TagOperationResult{}
	if err := query.ValidateTags(tags); err != nil {
		return result, err
	}
	analysis := &fileStateAnalysis{
		allFileData:       make([]fileData, 0, len(files)),
		allHashes:         make([]string, 0, len(files)),
		locationsToUpsert: make(map[string]types.LocationInfo, len(files)),
		potentialMoves:    make(map[string]string),
	}
	for _, file := range files {
		if file.Path == "" || file.Hash == "" {
			if progressCb != nil {
				progressCb(file.Path, fmt.Errorf("file path and hash are required"))
			}
			continue
		}
		analysis.allFileData = append(analysis.allFileData, fileData{path: file.Path, info: file})
		analysis.allHashes = append(analysis.allHashes, file.Hash)
		analysis.locationsToUpsert[file.Path] = file
	}
	if len(analysis.allFileData) == 0 {
		return result, nil
	}
	affectedCount, _, err := c.executeTaggingTransaction(analysis, tags, opTag, tasks, stateBuilder, nil)
	if err != nil {
		return result, err
	}
	result.AffectedCount = int(affectedCount)
	if progressCb != nil {
		for _, data := range analysis.allFileData {
			progressCb(data.path, nil)
		}
	}
	return result, nil
}

// UntagFiles removes tags from files. If no tags are provided, all tags are removed.
// Returns the number of tag associations removed and a list of notifications.
func (c *Client) UntagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadataHeuristic bool) (types.TagOperationResult, error) {
	if err := query.ValidateTags(tags); err != nil {
		return types.TagOperationResult{}, err
	}
	if len(tags) == 0 {
		// Clearing all tags is equivalent to `settags` with no tags.
		// For settags, we'll return the number of tags cleared.
		return c.performTagOperation(filePaths, []string{}, progressCb, opSetTags, useMetadataHeuristic)
	}
	return c.performTagOperation(filePaths, tags, progressCb, opUntag, useMetadataHeuristic)
}

// TagFilesByQuery adds tags to all files matching a query expression.
// Returns the number of tags added (which may be different from files affected if tags already existed).
func (c *Client) TagFilesByQuery(expression string, tags []string) (int, error) {
	if err := query.ValidateTags(tags); err != nil {
		return 0, err
	}
	if len(tags) == 0 {
		return 0, nil
	}
	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return 0, err
	}
	if sqlQuery == "" {
		return 0, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 1. Get or create the necessary tags to get their IDs.
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
	if err != nil {
		return 0, fmt.Errorf("failed to get or create tags: %w", err)
	}

	tagIDs := make([]int64, 0, len(tagIDMap))
	for _, id := range tagIDMap {
		tagIDs = append(tagIDs, id)
	}

	// 2. Directly associate tags with the content matching the query.
	// This is a pure-SQL operation that avoids loading all hashes into application memory.
	affected, err := c.store.BatchAssociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
	if err != nil {
		return 0, err
	}

	return int(affected), tx.Commit()
}

// UntagFilesByQuery removes tags from all files matching a query expression.
// If tags is empty, it removes ALL tags from matching files and returns the number of files affected.
// Otherwise, it returns the number of tag associations removed.
func (c *Client) UntagFilesByQuery(expression string, tags []string) (int, error) {
	// The query parser validates the expression syntax. We only need to validate the
	// separate `tags` argument if it's provided.
	if len(tags) > 0 {
		if err := query.ValidateTags(tags); err != nil {
			return 0, err
		}
	}

	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return 0, err
	}
	if sqlQuery == "" {
		return 0, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if len(tags) == 0 {
		// Clear all tags for content matching the query.
		// We need to count the number of affected files *before* we delete the tags,
		// as the deletion would cause the query to no longer find them.
		countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (%s)`, sqlQuery)
		var fileCount int
		if err := tx.QueryRow(countQuery, args...).Scan(&fileCount); err != nil {
			return 0, fmt.Errorf("failed to count files for tag clearing: %w", err)
		}
		if fileCount == 0 {
			return 0, tx.Commit()
		}

		if _, err := c.store.BatchClearTagsByContentQueryTx(tx, sqlQuery, args); err != nil {
			return 0, fmt.Errorf("failed to clear tags: %w", err)
		}
		return fileCount, tx.Commit()
	}

	// Untag specific tags from content matching the query.
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	// Use BatchGetTags as we don't want to create tags that don't exist.
	tagIDMap, err := c.store.BatchGetTags(tx, parsedTags)
	if err != nil {
		return 0, fmt.Errorf("failed to look up tags: %w", err)
	}
	if len(tagIDMap) == 0 {
		return 0, tx.Commit() // No matching tags found in the DB to remove.
	}

	tagIDs := make([]int64, 0, len(tagIDMap))
	for _, id := range tagIDMap {
		tagIDs = append(tagIDs, id)
	}

	affected, err := c.store.BatchDisassociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
	if err != nil {
		return 0, err
	}

	return int(affected), tx.Commit()
}

// SetTagsForFilesByQuery sets tags for all files matching a query expression, replacing existing ones.
// Returns the number of content items affected.
func (c *Client) SetTagsForFilesByQuery(expression string, tags []string) (int, error) {
	if err := query.ValidateTags(tags); err != nil {
		return 0, err
	}
	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return 0, err
	}
	if sqlQuery == "" {
		return 0, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// To perform a "set" operation correctly, we must get a static list of the
	// content that matches the query *before* any modifications are made.
	// We use a temporary table that exists only for this transaction to hold this list.
	// This avoids loading a potentially huge list of hashes into application memory.
	tempTableQuery := fmt.Sprintf("CREATE TEMP TABLE hashes_to_update AS %s", sqlQuery)
	if _, err := tx.Exec(tempTableQuery, args...); err != nil {
		return 0, fmt.Errorf("failed to create temporary table for update: %w", err)
	}

	// Get the count of affected files for the return value.
	var affectedCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM hashes_to_update").Scan(&affectedCount); err != nil {
		// The temp table will be dropped automatically on rollback.
		return 0, fmt.Errorf("failed to count files for update: %w", err)
	}

	if affectedCount == 0 {
		return 0, tx.Commit() // Commit will drop the (empty) temp table.
	}

	// 2. Clear existing tags for this static list of hashes.
	if _, err := tx.Exec("DELETE FROM content_tags WHERE content_hash IN (SELECT hash FROM hashes_to_update)"); err != nil {
		return 0, fmt.Errorf("failed to clear existing tags: %w", err)
	}

	// 3. Add new tags for the same static list of hashes.
	if len(tags) > 0 {
		parsedTags := make([]types.ParsedTag, len(tags))
		for i, t := range tags {
			parsedTags[i] = query.ParseTag(t)
		}
		tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
		if err != nil {
			return 0, fmt.Errorf("failed to get or create new tags: %w", err)
		}

		tagIDs := make([]int64, 0, len(tagIDMap))
		for _, id := range tagIDMap {
			tagIDs = append(tagIDs, id)
		}

		if _, err := c.store.BatchAssociateTagsByContentQueryTx(tx, "SELECT hash FROM hashes_to_update", nil, tagIDs); err != nil {
			return 0, fmt.Errorf("failed to associate new tags: %w", err)
		}
	}

	return affectedCount, tx.Commit()
}

// RenameTag renames an existing tag to a new name across the entire database.
// The new name must not already exist.
func (c *Client) RenameTag(oldName, newName string) error {
	// The new tag name must be valid for creation.
	if err := query.ValidateTag(newName); err != nil {
		return fmt.Errorf("invalid new tag name: %w", err)
	}
	// The old tag name must have valid syntax. Building a minimal AST and validating
	// it is the public way to check syntax without checking for reserved keywords.
	if err := query.ValidateAST(&query.Expression{Or: []*query.AndTerm{{And: []*query.Term{{Factor: &query.Factor{Tag: &oldName}}}}}}); err != nil {
		return fmt.Errorf("invalid old tag name: %w", err)
	}
	if oldName == newName {
		return fmt.Errorf("old and new tag names are identical")
	}

	oldParsedTag := query.ParseTag(oldName)
	newParsedTag := query.ParseTag(newName)

	return c.store.RenameTag(oldParsedTag, newParsedTag)
}

// SetTagsForFiles sets the tags for multiple files, replacing any existing ones, using a batching strategy.
// Returns the total number of changes (associations removed + associations added) and a list of notifications.
func (c *Client) SetTagsForFiles(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadataHeuristic bool) (types.TagOperationResult, error) {
	if err := query.ValidateTags(tags); err != nil {
		return types.TagOperationResult{}, err
	}
	return c.performTagOperation(filePaths, tags, progressCb, opSetTags, useMetadataHeuristic)
}

// fileData is an internal struct for tracking file state during a tagging operation.
type fileData struct {
	path         string // original path for callbacks
	info         types.LocationInfo
	wasModified  bool
	orphanedTags []string
}

// fileStateAnalysis holds the results of analyzing the initial state of files before a transaction.
type fileStateAnalysis struct {
	allFileData       []fileData
	allHashes         []string
	locationsToUpsert map[string]types.LocationInfo
	potentialMoves    map[string]string // hash -> newPath
}

// analyzeFileStates performs all filesystem and hashing operations to prepare for a tagging transaction.
func (c *Client) analyzeFileStates(filePaths []string, progressCb func(filePath string, err error), useMetadataHeuristic bool) (*fileStateAnalysis, error) {
	analysis := &fileStateAnalysis{
		locationsToUpsert: make(map[string]types.LocationInfo),
		potentialMoves:    make(map[string]string),
	}
	processedPaths := make(map[string]bool)

	// 1a. Resolve paths and collect absolute paths.
	absPaths := make([]string, 0, len(filePaths))
	originalPathMap := make(map[string]string, len(filePaths))
	for _, fp := range filePaths {
		absPath, err := resolvePath(fp)
		if err != nil {
			if progressCb != nil {
				progressCb(fp, err)
			}
			continue
		}
		absPaths = append(absPaths, absPath)
		originalPathMap[absPath] = fp
	}

	// 1b. Get existing location data from the DB.
	dbLocations, err := c.store.BatchGetLocationsByPaths(absPaths)
	if err != nil {
		return nil, fmt.Errorf("could not get existing file data: %w", err)
	}

	// 1c. Determine which files actually need to be hashed. Cache the initial
	// metadata so unchanged heuristic hits do not need a second Stat in phase 2.
	initialFileInfo := make(map[string]os.FileInfo, len(absPaths))
	filesToHash := make([]string, 0)
	for _, absPath := range absPaths {
		info, err := os.Stat(absPath)
		if err != nil {
			// Report the initial Stat failure once. Phase 2 still retries the Stat
			// so a path that becomes available can continue through analysis.
			originalPath := originalPathMap[absPath]
			if progressCb != nil {
				progressCb(originalPath, err)
			}
			processedPaths[originalPath] = true
			continue
		}
		initialFileInfo[absPath] = info
		// If using the heuristic, only hash if metadata differs. Otherwise, hash everything.
		if useMetadataHeuristic {
			dbInfo, existsInDb := dbLocations[absPath]
			if existsInDb && info.Size() == dbInfo.Size && info.ModTime().Unix() == dbInfo.ModTime {
				continue // Unchanged, no hash needed.
			}
		}
		filesToHash = append(filesToHash, absPath)
	}

	// 1d. Concurrently hash all the necessary files.
	hashResults := c.hasher.ConcurrentlyHashFiles(filesToHash)

	// Phase 2: Prepare data structures for transaction.
	for _, absPath := range absPaths {
		originalPath := originalPathMap[absPath]
		result, wasHashed := hashResults[absPath]
		info := initialFileInfo[absPath]
		if wasHashed || info == nil {
			// Hashed paths need post-hash metadata. A missing cached value means
			// phase 1 could not Stat the path, so retry it here.
			var err error
			info, err = os.Stat(absPath)
			if err != nil {
				if !processedPaths[originalPath] {
					if progressCb != nil {
						progressCb(originalPath, err)
					}
					processedPaths[originalPath] = true
				}
				continue
			}
		}

		var hash string
		dbInfo, existsInDb := dbLocations[absPath]
		isModification := false

		// Determine the definitive hash for the current file content.
		if wasHashed {
			if result.Err != nil {
				if !processedPaths[originalPath] {
					errMsg := fmt.Sprintf("hashing failed: %v", result.Err)
					if progressCb != nil {
						progressCb(originalPath, fmt.Errorf("%s", errMsg))
					}
					processedPaths[originalPath] = true
				}
				continue
			}
			hash = result.Hash
		} else {
			// This path was not in filesToHash, meaning it's unchanged according to the heuristic.
			// We can trust the DB hash.
			if !existsInDb {
				// Should be logically impossible to get here, but handle defensively.
				continue
			}
			hash = dbInfo.Hash
		}

		if existsInDb && hash != dbInfo.Hash {
			isModification = true
		}

		if wasHashed {
			analysis.potentialMoves[hash] = absPath
		}

		var orphanedTags []string
		if isModification {
			oldTags, err := c.store.GetTagsForContent(dbInfo.Hash)
			if err != nil && err != sql.ErrNoRows {
				// Log? For now, just proceed.
			} else {
				orphanedTags = oldTags
			}
		}

		locInfo := types.LocationInfo{
			Path:      absPath,
			Hash:      hash,
			Size:      info.Size(),
			ModTime:   info.ModTime().Unix(),
			Extension: filepath.Ext(absPath),
		}
		analysis.allFileData = append(analysis.allFileData, fileData{
			path:         originalPath,
			info:         locInfo,
			wasModified:  isModification,
			orphanedTags: orphanedTags,
		})
		analysis.allHashes = append(analysis.allHashes, hash)
		analysis.locationsToUpsert[absPath] = locInfo
	}

	return analysis, nil
}

// applyTaggingOperationInTx contains the switch logic for tag, settags, and untag.
func (c *Client) applyTaggingOperationInTx(tx *database.Tx, hashes []string, tags []string, kind opKind) (int64, error) {
	var affectedCount int64
	switch kind {
	case opTag:
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
			if err != nil {
				return 0, fmt.Errorf("failed to get or create tags: %w", err)
			}
			pairs := make([]database.ContentTagPair, 0, len(hashes)*len(tags))
			for _, hash := range hashes {
				for _, tagStr := range tags {
					pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagIDMap[tagStr]})
				}
			}
			affected, err := c.store.BatchAssociateTags(tx, pairs)
			if err != nil {
				return 0, fmt.Errorf("failed to batch associate tags: %w", err)
			}
			affectedCount = affected
		}
	case opSetTags:
		cleared, err := c.store.BatchClearTagsForContent(tx, hashes)
		if err != nil {
			return 0, fmt.Errorf("failed to batch clear tags: %w", err)
		}
		var associated int64
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
			if err != nil {
				return 0, fmt.Errorf("failed to get or create tags: %w", err)
			}
			pairs := make([]database.ContentTagPair, 0, len(hashes)*len(tags))
			for _, hash := range hashes {
				for _, tagStr := range tags {
					pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagIDMap[tagStr]})
				}
			}
			associated, err = c.store.BatchAssociateTags(tx, pairs)
			if err != nil {
				return 0, fmt.Errorf("failed to batch associate tags: %w", err)
			}
		}
		affectedCount = cleared + associated
	case opUntag:
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetTags(tx, parsedTags)
			if err != nil {
				return 0, fmt.Errorf("failed to look up tags: %w", err)
			}
			pairs := make([]database.ContentTagPair, 0, len(hashes)*len(tags))
			for _, hash := range hashes {
				for _, tagStr := range tags {
					if tagID, ok := tagIDMap[tagStr]; ok {
						pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagID})
					}
				}
			}
			affected, err := c.store.BatchDisassociateTags(tx, pairs)
			if err != nil {
				return 0, fmt.Errorf("failed to batch disassociate tags: %w", err)
			}
			affectedCount = affected
		}
	}
	return affectedCount, nil
}

func (c *Client) persistTaggingFollowUpInTx(tx *databaseTx, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, affectedCount int64) error {
	_, err := c.persistTaggingFollowUpTrackedInTx(tx, tasks, stateBuilder, affectedCount)
	return err
}

func (c *Client) persistTaggingFollowUpTrackedInTx(tx *databaseTx, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, affectedCount int64) (bool, error) {
	operationChanged := false
	for _, task := range tasks {
		backgroundTask, created, err := c.enqueueBackgroundTask(tx, task)
		if err != nil {
			return false, fmt.Errorf("failed to enqueue background task: %w", err)
		}
		if created && backgroundTask.OperationID != "" {
			operationChanged = true
		}
	}
	if stateBuilder == nil {
		return operationChanged, nil
	}
	state, err := stateBuilder(int(affectedCount))
	if err != nil {
		return false, fmt.Errorf("build background operation transaction state: %w", err)
	}
	if state.OperationID == "" && state.TaskID == "" {
		return false, errors.New("background transaction state requires an operation or task id")
	}
	checkpointJSON, err := json.Marshal(state.Checkpoint)
	if err != nil {
		return false, fmt.Errorf("encode background transaction checkpoint: %w", err)
	}
	resultJSON, err := json.Marshal(state.Result)
	if err != nil {
		return false, fmt.Errorf("encode background transaction result: %w", err)
	}
	if state.TaskID != "" {
		if err := setDatabaseBackgroundTaskState(c, tx, state.TaskID, checkpointJSON, resultJSON); err != nil {
			return false, fmt.Errorf("persist background task transaction state: %w", err)
		}
	}
	if state.OperationID != "" {
		if err := setDatabaseBackgroundOperationState(c, tx, state.OperationID, checkpointJSON, resultJSON); err != nil {
			return false, fmt.Errorf("persist background operation transaction state: %w", err)
		}
		operationChanged = true
	}
	return operationChanged, nil
}

// executeTaggingTransaction performs all database writes for a tagging operation.
func (c *Client) executeTaggingTransaction(analysis *fileStateAnalysis, tags []string, kind opKind, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, finalizer taggingTransactionFinalizer) (int64, map[string]string, error) {
	movesHandled := make(map[string]string) // newPath -> oldPath
	tx, err := c.store.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	operationChanged := false

	// 1. Intelligently handle file moves.
	if len(analysis.potentialMoves) > 0 {
		hashesToCheck := make([]string, 0, len(analysis.potentialMoves))
		for hash := range analysis.potentialMoves {
			hashesToCheck = append(hashesToCheck, hash)
		}
		hashToOldPaths, err := c.store.BatchGetPathsForHashes(tx, hashesToCheck)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to check for existing content paths: %w", err)
		}
		for hash, oldPaths := range hashToOldPaths {
			newPath := analysis.potentialMoves[hash]
			for _, oldPath := range oldPaths {
				if oldPath == newPath {
					continue
				}
				if _, statErr := os.Stat(oldPath); os.IsNotExist(statErr) {
					newLocationInfo := analysis.locationsToUpsert[newPath]
					if err := c.store.UpdateMovedLocation(tx, oldPath, newLocationInfo); err != nil {
						return 0, nil, fmt.Errorf("failed to update moved path from '%s' to '%s': %w", oldPath, newPath, err)
					}
					delete(analysis.locationsToUpsert, newPath)
					movesHandled[newPath] = oldPath
					break
				}
			}
		}
	}

	// 2. Batch upsert contents and locations.
	if err := c.store.BatchInsertContents(tx, analysis.allHashes); err != nil {
		return 0, nil, fmt.Errorf("failed to batch insert contents: %w", err)
	}
	if err := c.store.BatchUpsertLocations(tx, analysis.locationsToUpsert); err != nil {
		return 0, nil, fmt.Errorf("failed to batch upsert locations: %w", err)
	}

	// 3. Perform the specific tagging operation.
	affectedCount, err := c.applyTaggingOperationInTx(tx, analysis.allHashes, tags, kind)
	if err != nil {
		return 0, nil, err
	}

	// 4. Build registration-hook work and persist it together with any caller-owned
	// durable follow-up work in the same transaction as content registration.
	hookTasks, err := c.fileRegistrationBackgroundTasks(analysis.allHashes)
	if err != nil {
		return 0, nil, fmt.Errorf("build file registration background tasks: %w", err)
	}
	tasks = append(tasks, hookTasks...)
	operationChanged, err = c.persistTaggingFollowUpTrackedInTx(tx, tasks, stateBuilder, affectedCount)
	if err != nil {
		return 0, nil, err
	}

	if finalizer != nil {
		if err := finalizer(tx, affectedCount, movesHandled); err != nil {
			return 0, nil, fmt.Errorf("finalize tagging transaction: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	if operationChanged {
		c.notifyBackgroundOperationChange()
	}
	return affectedCount, movesHandled, nil
}

// performTagOperation is the refactored, high-level coordinator for all path-based tagging.
func (c *Client) performTagOperation(filePaths []string, tags []string, progressCb func(filePath string, err error), kind opKind, useMetadataHeuristic bool) (types.TagOperationResult, error) {
	result := types.TagOperationResult{}

	// Phase 1 & 2: Analyze file states (FS interactions and hashing).
	analysis, err := c.analyzeFileStates(filePaths, progressCb, useMetadataHeuristic)
	if err != nil {
		return result, err
	}

	if len(analysis.allFileData) == 0 {
		return result, nil // No files could be processed.
	}

	// Phase 3: The Transaction (all DB writes).
	affectedCount, movesHandled, err := c.executeTaggingTransaction(analysis, tags, kind, nil, nil, nil)
	if err != nil {
		return result, err
	}
	result.AffectedCount = int(affectedCount)

	// Phase 4: Build Notifications and call Progress Callback.
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

	if progressCb != nil {
		for _, data := range analysis.allFileData {
			progressCb(data.path, nil)
		}
	}

	return result, nil
}
