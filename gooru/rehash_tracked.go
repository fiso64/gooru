package gooru

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"gooru.local/types"
)

// RehashTrackedFiles is the managed-storage-aware rehash path used by the CLI.
// The argument remains the canonical logical location stored in the database;
// when that location is backed by managed storage, hashing reads the separate
// physical path without changing the logical identity.
func (c *Client) RehashTrackedFiles(filePaths []string, progressCb func(path string, status types.RehashStatus, err error), useMetadataHeuristic bool) {
	for _, originalPath := range filePaths {
		logicalPath, err := resolvePath(originalPath)
		if err != nil {
			progressCb(originalPath, 0, err)
			continue
		}

		dbInfo, err := c.store.GetLocationSourceByPath(logicalPath)
		if err != nil {
			if err == sql.ErrNoRows {
				progressCb(originalPath, types.StatusSkippedNotInDB, nil)
			} else {
				progressCb(originalPath, 0, fmt.Errorf("database lookup failed: %w", err))
			}
			continue
		}

		sourcePath := logicalPath
		if dbInfo.StoragePath != "" {
			sourcePath = dbInfo.StoragePath
		}

		logicalInfo, err := c.hasher.FileMetadata(sourcePath)
		if err != nil {
			progressCb(originalPath, 0, err)
			continue
		}

		if useMetadataHeuristic && logicalInfo.Size == dbInfo.Size && logicalInfo.ModTime.Unix() == dbInfo.ModTime {
			progressCb(originalPath, types.StatusSkippedUnchanged, nil)
			continue
		}

		newHash, err := c.hasher.HashFile(sourcePath)
		if err != nil {
			progressCb(originalPath, 0, fmt.Errorf("hashing failed: %w", err))
			continue
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
			continue
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
			continue
		}
		progressCb(originalPath, types.StatusRehashed, nil)
	}
}
