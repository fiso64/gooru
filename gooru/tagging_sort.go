package gooru

import (
	"fmt"
	"os"
	"time"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// FileRegistrationSort controls how newly tracked paths are ordered by added time.
type FileRegistrationSort string

const (
	// FileRegistrationSortQueue makes later input paths newer within one batch.
	FileRegistrationSortQueue FileRegistrationSort = "queue"
	// FileRegistrationSortReverseQueue makes earlier input paths newer within one batch.
	FileRegistrationSortReverseQueue FileRegistrationSort = "reverse_queue"
	// FileRegistrationSortModTime uses each file's filesystem modification time.
	FileRegistrationSortModTime FileRegistrationSort = "modtime"
)

func validateFileRegistrationSort(sort FileRegistrationSort) error {
	switch sort {
	case FileRegistrationSortQueue, FileRegistrationSortReverseQueue, FileRegistrationSortModTime:
		return nil
	default:
		return fmt.Errorf("registration sort must be one of: queue, reverse_queue, modtime")
	}
}

// TagFilesWithSort behaves like TagFiles, but controls added ordering for paths
// that become tracked during this operation. Already tracked paths and detected
// moves keep their existing added time/order.
func (c *Client) TagFilesWithSort(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadataHeuristic bool, sort FileRegistrationSort) (types.TagOperationResult, error) {
	if err := query.ValidateTags(tags); err != nil {
		return types.TagOperationResult{}, err
	}
	return c.performTagOperationWithSort(filePaths, tags, progressCb, opTag, useMetadataHeuristic, sort)
}

// SetTagsForFilesWithSort behaves like SetTagsForFiles, but controls added
// ordering for paths that become tracked during this operation.
func (c *Client) SetTagsForFilesWithSort(filePaths []string, tags []string, progressCb func(filePath string, err error), useMetadataHeuristic bool, sort FileRegistrationSort) (types.TagOperationResult, error) {
	if err := query.ValidateTags(tags); err != nil {
		return types.TagOperationResult{}, err
	}
	return c.performTagOperationWithSort(filePaths, tags, progressCb, opSetTags, useMetadataHeuristic, sort)
}

type registrationSortValue struct {
	addedAt    int64
	addedOrder int64
}

func registrationSortValues(analysis *fileStateAnalysis, sort FileRegistrationSort, batchTime time.Time) map[string]registrationSortValue {
	values := make(map[string]registrationSortValue, len(analysis.allFileData))
	total := len(analysis.allFileData)
	for index, data := range analysis.allFileData {
		addedAt := batchTime.UnixMilli()
		addedOrder := int64(index)
		switch sort {
		case FileRegistrationSortReverseQueue:
			addedOrder = int64(total - 1 - index)
		case FileRegistrationSortModTime:
			addedOrder = 0
			if info, err := os.Stat(data.info.Path); err == nil {
				addedAt = info.ModTime().UnixMilli()
			}
		}
		values[data.info.Path] = registrationSortValue{addedAt: addedAt, addedOrder: addedOrder}
	}
	return values
}

func (c *Client) performTagOperationWithSort(filePaths []string, tags []string, progressCb func(filePath string, err error), kind opKind, useMetadataHeuristic bool, sort FileRegistrationSort) (types.TagOperationResult, error) {
	result := types.TagOperationResult{}
	if err := validateFileRegistrationSort(sort); err != nil {
		return result, err
	}
	batchTime := time.Now()

	analysis, err := c.analyzeFileStates(filePaths, progressCb, useMetadataHeuristic)
	if err != nil {
		return result, err
	}
	if len(analysis.allFileData) == 0 {
		return result, nil
	}

	paths := make([]string, 0, len(analysis.locationsToUpsert))
	for path := range analysis.locationsToUpsert {
		paths = append(paths, path)
	}
	existingLocations, err := c.store.BatchGetLocationsByPaths(paths)
	if err != nil {
		return result, fmt.Errorf("could not get existing file data for registration ordering: %w", err)
	}
	sortValues := registrationSortValues(analysis, sort, batchTime)

	finalizer := func(tx *databaseTx, _ int64, movesHandled map[string]string) error {
		for path, value := range sortValues {
			if _, existed := existingLocations[path]; existed {
				continue
			}
			if _, moved := movesHandled[path]; moved {
				continue
			}
			location, ok := analysis.locationsToUpsert[path]
			if !ok {
				continue
			}
			// Match the insertion timestamp assigned at the registration boundary.
			// If another writer registered this path first, the upsert preserves that
			// row's older added_at and this update deliberately becomes a no-op.
			if _, err := tx.Exec(`UPDATE locations SET added_at = ?, added_order = ? WHERE path = ? AND added_at = ?`, value.addedAt, value.addedOrder, path, location.AddedAt); err != nil {
				return fmt.Errorf("apply registration sort for %q: %w", path, err)
			}
		}
		return nil
	}

	affectedCount, movesHandled, err := c.executeTaggingTransaction(analysis, tags, kind, nil, nil, finalizer)
	if err != nil {
		return result, err
	}
	result.AffectedCount = int(affectedCount)

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
