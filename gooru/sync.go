package gooru

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gooru.local/types"
	"gooru.local/internal/scanning"
)

// DeleteFilesByQuery removes file records from the database that match a query expression.
// This permanently removes the location records and any associated content/tags if they become orphaned.
// Returns the number of location records removed.
func (c *Client) DeleteFilesByQuery(expression string) (int, error) {
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

	// The trigger `cleanup_orphan_content_on_delete` will handle cleaning up content
	// and tags if all locations for a piece of content are removed.
	affected, err := c.store.RemoveLocationsByContentQueryTx(tx, sqlQuery, args)
	if err != nil {
		return 0, err
	}

	return int(affected), tx.Commit()
}

// EditPath manually updates a file's path in the database.
func (c *Client) EditPath(oldPath, newPath string) error {
	// The database layer needs absolute paths for consistency,
	// but we pass the original paths as well for clearer error messages.
	absOldPath, err := resolvePath(oldPath)
	if err != nil {
		return fmt.Errorf("could not resolve old path '%s': %w", oldPath, err)
	}

	// For a manual edit, we must get the metadata from the new file on disk.
	fsInfo, err := os.Stat(newPath)
	if err != nil {
		return fmt.Errorf("could not stat new path '%s': %w", newPath, err)
	}

	absNewPath, err := resolvePath(newPath)
	if err != nil {
		return fmt.Errorf("could not resolve new path '%s': %w", newPath, err)
	}

	newInfo := types.LocationInfo{
		Path:      absNewPath,
		Size:      fsInfo.Size(),
		ModTime:   fsInfo.ModTime().Unix(),
		Extension: filepath.Ext(absNewPath),
	}

	return c.store.UpdatePath(absOldPath, newInfo, oldPath, newPath)
}

// NeedsRelink performs a fast check using filesystem metadata to see if a relink is necessary.
func (c *Client) NeedsRelink(dirs []string) (bool, error) {
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return false, err
	}
	dbLocations, err := c.store.GetLocationsForDirs(absDirs)
	if err != nil {
		return false, err
	}

	for path, info := range dbLocations {
		fsInfo, err := os.Stat(path)
		if os.IsNotExist(err) {
			return true, nil // File in DB is missing from disk.
		}
		if err != nil {
			return true, nil // Can't stat the file, something is wrong.
		}

		if fsInfo.Size() != info.Size || fsInfo.ModTime().Unix() != info.ModTime {
			return true, nil // Metadata mismatch, file has likely changed.
		}
	}

	return false, nil // Everything matches.
}

// Relink performs a "dry run" scan to find proposed changes between the database and the filesystem.
func (c *Client) Relink(dirs []string) (types.RelinkResult, error) {
	result := types.RelinkResult{}
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return result, err
	}

	// 1. Get initial state from the database.
	dbLocations, err := c.store.GetLocationsForDirs(absDirs)
	if err != nil {
		return result, fmt.Errorf("could not get db locations: %w", err)
	}
	sizeToHashes, err := c.store.GetSizeToHashesMap()
	if err != nil {
		return result, fmt.Errorf("could not build size-to-hash map: %w", err)
	}

	// 2. Perform the intelligent, targeted filesystem scan.
	fsLocations, filesScanned := scanning.DirsConcurrently(absDirs, sizeToHashes, c.hasher)
	result.Stats.FilesScanned = filesScanned

	// 3. Reconcile states.
	handledDbPaths := make(map[string]bool)
	fsHashToPaths := make(map[string][]string)
	for path, info := range fsLocations {
		fsHashToPaths[info.Hash] = append(fsHashToPaths[info.Hash], path)
	}

	// 3a. Find moves and unchanged files.
dbPathLoop:
	for dbPath, dbInfo := range dbLocations {
		// Check for unchanged files first
		fsInfo, existsOnFs := fsLocations[dbPath]
		if existsOnFs && fsInfo.Hash == dbInfo.Hash {
			handledDbPaths[dbPath] = true
			delete(fsLocations, dbPath) // This fs location is accounted for
			continue
		}

		// Check if content has moved
		if newPaths, contentExistsOnFs := fsHashToPaths[dbInfo.Hash]; contentExistsOnFs {
			for i, newPath := range newPaths {
				if _, isHandled := fsLocations[newPath]; !isHandled {
					continue // This path was an unchanged file or already used for a move.
				}

				// Get the full info for the new location from the scan results.
				newLocationInfo, locationFound := fsLocations[newPath]
				if !locationFound {
					// This should be logically impossible due to the preceding checks,
					// but handle defensively.
					continue
				}

				result.ProposedMoves = append(result.ProposedMoves, types.MoveInfo{
					OldPath:     dbPath,
					NewLocation: newLocationInfo,
				})
				handledDbPaths[dbPath] = true
				delete(fsLocations, newPath)       // This fs location is accounted for
				fsHashToPaths[dbInfo.Hash][i] = "" // Mark this path as used
				continue dbPathLoop                // Move to the next db path
			}
		}
	}

	// 3b. Any remaining fsLocations are new locations for existing content (duplicates).
	for _, fsInfo := range fsLocations {
		result.ProposedAdds = append(result.ProposedAdds, fsInfo)
	}

	// 3c. Any unhandled dbLocations are genuine deletions.
	for path, dbInfo := range dbLocations {
		if !handledDbPaths[path] {
			result.ProposedDeletes = append(result.ProposedDeletes, types.FileInfo{
				Path: path,
				Size: dbInfo.Size,
				Tags: dbInfo.TagsCache,
			})
		}
	}

	return result, nil
}

// ApplyRelinkChanges executes the changes proposed by a Relink dry run.
func (c *Client) ApplyRelinkChanges(changes types.RelinkResult) (types.RelinkStats, error) {
	stats := types.RelinkStats{}
	if len(changes.ProposedMoves) == 0 && len(changes.ProposedAdds) == 0 && len(changes.ProposedDeletes) == 0 {
		return stats, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	// 1. Apply moves
	for _, move := range changes.ProposedMoves {
		if err := c.store.UpdateMovedLocation(tx, move.OldPath, move.NewLocation); err != nil {
			return stats, fmt.Errorf("failed to update moved path from '%s' to '%s': %w", move.OldPath, move.NewLocation.Path, err)
		}
	}
	// A move counts as an update. We'll add it to LocationsAdded for a combined stat.
	stats.LocationsAdded += len(changes.ProposedMoves)

	// 2. Apply additions
	if len(changes.ProposedAdds) > 0 {
		toAdd := make(map[string]types.LocationInfo)
		for _, add := range changes.ProposedAdds {
			toAdd[add.Path] = add
		}

		addedCount, err := c.store.ApplyRelinkAdditionsTx(tx, toAdd)
		if err != nil {
			return stats, fmt.Errorf("failed to apply additions: %w", err)
		}
		stats.LocationsAdded += addedCount
	}

	// 3. Apply deletions
	if len(changes.ProposedDeletes) > 0 {
		pathsToDelete := make([]string, len(changes.ProposedDeletes))
		for i, del := range changes.ProposedDeletes {
			pathsToDelete[i] = del.Path
		}
		removedCount, err := c.store.RemoveLocationsByPathTx(tx, pathsToDelete)
		if err != nil {
			return stats, fmt.Errorf("failed to apply deletions: %w", err)
		}
		stats.LocationsRemoved = removedCount
	}

	if err := tx.Commit(); err != nil {
		return stats, err
	}

	return stats, nil
}

// PruneLocations removes a list of file paths from the database.
func (c *Client) PruneLocations(paths []string) (int, error) {
	return c.store.RemoveLocationsByPath(paths)
}

// RehashFiles updates the content record for files that have been modified on disk, preserving their tags.
func (c *Client) RehashFiles(filePaths []string, progressCb func(path string, status types.RehashStatus, err error)) {
	for _, originalPath := range filePaths {
		absPath, err := resolvePath(originalPath)
		if err != nil {
			progressCb(originalPath, 0, err)
			continue
		}

		dbInfo, err := c.store.GetLocationByPath(absPath)
		if err != nil {
			if err == sql.ErrNoRows {
				progressCb(originalPath, types.StatusSkippedNotInDB, nil)
			} else {
				progressCb(originalPath, 0, fmt.Errorf("database lookup failed: %w", err))
			}
			continue
		}

		fsInfo, err := os.Stat(absPath)
		if err != nil {
			progressCb(originalPath, 0, err) // e.g., file deleted from disk
			continue
		}

		if fsInfo.Size() == dbInfo.Size && fsInfo.ModTime().Unix() == dbInfo.ModTime {
			progressCb(originalPath, types.StatusSkippedUnchanged, nil)
			continue
		}

		// File has been modified, proceed with rehash.
		newHash, err := c.hasher.HashFile(absPath)
		if err != nil {
			progressCb(originalPath, 0, fmt.Errorf("hashing failed: %w", err))
			continue
		}

		// Edge case: metadata changed, but content is identical. Just update metadata.
		if newHash == dbInfo.Hash {
			err := c.store.UpdateLocationMetadata(absPath, fsInfo.Size(), fsInfo.ModTime().Unix())
			if err != nil {
				progressCb(originalPath, 0, fmt.Errorf("metadata update failed: %w", err))
			} else {
				progressCb(originalPath, types.StatusMetadataUpdated, nil)
			}
			continue
		}

		// Full rehash: content has changed, transfer tags.
		newLocInfo := types.LocationInfo{
			Path:      absPath,
			Hash:      newHash,
			Size:      fsInfo.Size(),
			ModTime:   fsInfo.ModTime().Unix(),
			Extension: filepath.Ext(absPath),
		}
		err = c.store.TransferTagsAndRehashLocation(dbInfo.Hash, newHash, newLocInfo)
		if err != nil {
			progressCb(originalPath, 0, fmt.Errorf("transaction failed: %w", err))
		} else {
			progressCb(originalPath, types.StatusRehashed, nil)
		}
	}
}