package gooru

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"gooru.local/types"
)

const rehashLookupBatchSize = 512

type rehashLookupInput struct {
	originalPath string
	logicalPath  string
	resolveErr   error
	freshLookup  bool
}

// RehashTrackedFiles is the managed-storage-aware rehash path used by the CLI.
// The argument remains the canonical logical location stored in the database;
// when that location is backed by managed storage, hashing reads the separate
// physical path without changing the logical identity.
func (c *Client) RehashTrackedFiles(filePaths []string, progressCb func(path string, status types.RehashStatus, err error), useMetadataHeuristic bool) {
	for start := 0; start < len(filePaths); start += rehashLookupBatchSize {
		end := start + rehashLookupBatchSize
		if end > len(filePaths) {
			end = len(filePaths)
		}

		inputs := make([]rehashLookupInput, 0, end-start)
		lookupPaths := make([]string, 0, end-start)
		seenPaths := make(map[string]struct{}, end-start)
		for _, originalPath := range filePaths[start:end] {
			input := rehashLookupInput{originalPath: originalPath}
			input.logicalPath, input.resolveErr = resolvePath(originalPath)
			if input.resolveErr == nil {
				if _, seen := seenPaths[input.logicalPath]; seen {
					// Rehashing an earlier occurrence can mutate its hash or metadata.
					// Re-read duplicates when they are actually processed rather than
					// using source state captured before that mutation.
					input.freshLookup = true
				} else {
					seenPaths[input.logicalPath] = struct{}{}
					lookupPaths = append(lookupPaths, input.logicalPath)
				}
			}
			inputs = append(inputs, input)
		}

		var prefetched map[string]types.LocationInfo
		var prefetchErr error
		usePrefetch := len(lookupPaths) > 1
		if usePrefetch {
			prefetched, prefetchErr = c.store.BatchGetLocationSourcesByPaths(lookupPaths)
		}

		for _, input := range inputs {
			if input.resolveErr != nil {
				progressCb(input.originalPath, 0, input.resolveErr)
				continue
			}

			var dbInfo types.LocationInfo
			var err error
			if !usePrefetch || prefetchErr != nil || input.freshLookup {
				// Falling back after a batch error preserves the previous per-path
				// error/callback behavior instead of failing an entire read-ahead
				// window because one batched query could not be completed.
				dbInfo, err = c.store.GetLocationSourceByPath(input.logicalPath)
			} else {
				var ok bool
				dbInfo, ok = prefetched[input.logicalPath]
				if !ok {
					err = sql.ErrNoRows
				}
			}
			c.rehashTrackedFile(input.originalPath, input.logicalPath, dbInfo, err, progressCb, useMetadataHeuristic)
		}
	}
}

func (c *Client) rehashTrackedFile(originalPath, logicalPath string, dbInfo types.LocationInfo, lookupErr error, progressCb func(path string, status types.RehashStatus, err error), useMetadataHeuristic bool) {
	if lookupErr != nil {
		if lookupErr == sql.ErrNoRows {
			progressCb(originalPath, types.StatusSkippedNotInDB, nil)
		} else {
			progressCb(originalPath, 0, fmt.Errorf("database lookup failed: %w", lookupErr))
		}
		return
	}

	sourcePath := logicalPath
	if dbInfo.StoragePath != "" {
		sourcePath = dbInfo.StoragePath
	}

	logicalInfo, err := c.hasher.FileMetadata(sourcePath)
	if err != nil {
		progressCb(originalPath, 0, err)
		return
	}

	if useMetadataHeuristic && logicalInfo.Size == dbInfo.Size && logicalInfo.ModTime.Unix() == dbInfo.ModTime {
		progressCb(originalPath, types.StatusSkippedUnchanged, nil)
		return
	}

	newHash, err := c.hasher.HashFile(sourcePath)
	if err != nil {
		progressCb(originalPath, 0, fmt.Errorf("hashing failed: %w", err))
		return
	}

	if newHash == dbInfo.Hash {
		if logicalInfo.Size != dbInfo.Size || logicalInfo.ModTime.Unix() != dbInfo.ModTime {
			if err := c.store.UpdateLocationMetadata(logicalPath, logicalInfo.Size, logicalInfo.ModTime.Unix()); err != nil {
				progressCb(originalPath, 0, fmt.Errorf("metadata update failed: %w", err))
			} else {
				progressCb(originalPath, types.StatusMetadataUpdated, nil)
			}
		} else {
			progressCb(originalPath, types.StatusSkippedUnchanged, nil)
		}
		return
	}

	newLocInfo := types.LocationInfo{
		Path:        logicalPath,
		StoragePath: dbInfo.StoragePath,
		Hash:        newHash,
		Size:        logicalInfo.Size,
		ModTime:     logicalInfo.ModTime.Unix(),
		Extension:   filepath.Ext(logicalPath),
	}
	if err := c.store.RehashLocationPreservingTags(dbInfo.Hash, newHash, newLocInfo); err != nil {
		progressCb(originalPath, 0, fmt.Errorf("transaction failed: %w", err))
		return
	}
	progressCb(originalPath, types.StatusRehashed, nil)
}
