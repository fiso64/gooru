package gooru

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/internal/scanning"
	"gooru.local/types"
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

// NeedsRelink performs a check to see if a relink is necessary.
// By default, it uses a fast metadata check. If alwaysVerifyHash is true,
// it performs a slower but 100% accurate content hash check.
func (c *Client) NeedsRelink(dirs []string, alwaysVerifyHash bool) (bool, error) {
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return false, err
	}

	// Get DB state for the given directories.
	dbLocations, err := c.store.GetLocationsForDirs(absDirs)
	if err != nil {
		return false, err
	}

	// IMPORTANT: Get the size-to-hash map to filter the FS walk, exactly like the full Relink scan does.
	// This ensures both functions see the same set of "relevant" files on disk.
	sizeToHashes, err := c.store.GetSizeToHashesMap()
	if err != nil {
		return false, fmt.Errorf("could not build size-to-hash map for pre-check: %w", err)
	}

	// Get FS state for "relevant" files in the given directories.
	fsPaths := make(map[string]struct{})
	for _, dir := range absDirs {
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip unreadable files/dirs
			}
			if !d.IsDir() {
				info, err := d.Info()
				if err != nil {
					return nil // Skip files we can't stat
				}
				// The core filtering logic that must match Relink's scanner.
				if _, ok := sizeToHashes[info.Size()]; ok {
					fsPaths[path] = struct{}{}
				}
			}
			return nil
		})
		if walkErr != nil {
			return false, fmt.Errorf("failed to walk directory %s: %w", dir, walkErr)
		}
	}

	// Now that both fsPaths and dbLocations are looking at the same conceptual set of files,
	// the comparison logic will be correct.

	// 1. Fast check: If the number of files differs, a scan is definitely needed.
	if len(dbLocations) != len(fsPaths) {
		return true, nil
	}

	// 2. Slower check: Compare metadata for each file the DB expects to be there.
	for path, dbInfo := range dbLocations {
		// If a path from the DB is not in our filtered FS map, something is wrong (e.g., deleted).
		if _, ok := fsPaths[path]; !ok {
			return true, nil
		}

		fsInfo, err := os.Stat(path)
		if err != nil {
			return true, nil // File vanished between walk and stat, or permissions changed.
		}

		if alwaysVerifyHash {
			currentHash, err := c.hasher.HashFile(path)
			if err != nil {
				return true, nil // Can't hash the file, treat as changed.
			}
			if currentHash != dbInfo.Hash {
				return true, nil // Content hash mismatch.
			}
		} else {
			if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
				return true, nil // Metadata mismatch.
			}
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

	// --- Phase 1: Gather State ---
	dbLocationsInScope, err := c.store.GetLocationsForDirs(absDirs)
	if err != nil {
		return result, fmt.Errorf("could not get in-scope db locations: %w", err)
	}
	sizeToHashes, err := c.store.GetSizeToHashesMap()
	if err != nil {
		return result, fmt.Errorf("could not build size-to-hash map: %w", err)
	}
	fsLocations, filesScanned := scanning.DirsConcurrently(absDirs, sizeToHashes, c.hasher)
	result.Stats.FilesScanned = filesScanned

	fsHashesOnDisk := make(map[string]types.LocationInfo)
	for _, info := range fsLocations {
		if _, ok := fsHashesOnDisk[info.Hash]; !ok {
			fsHashesOnDisk[info.Hash] = info
		}
	}

	// --- Phase 2: Find Deletions and Moves-Within-Scope ---
	handledFsPaths := make(map[string]bool)

	for dbPath, dbInfo := range dbLocationsInScope {
		fsInfo, pathExistsOnFs := fsLocations[dbPath]

		if pathExistsOnFs {
			if fsInfo.Hash == dbInfo.Hash {
				// Case 1: Unchanged file.
				handledFsPaths[dbPath] = true
			} else {
				// Case 2: Modified file. Treat as a deletion of the old record.
				// The new file at this path will be handled in Phase 3.
				var tags []string
				if dbInfo.TagsCache != "" {
					tags = strings.Split(dbInfo.TagsCache, " ")
				}
				result.ProposedDeletes = append(result.ProposedDeletes, types.FileInfo{
					Path: dbPath, Size: dbInfo.Size, Tags: tags,
				})
			}
		} else {
			// Case 3: Path from DB is missing. Could be move-within-scope or deletion.
			if newLocation, contentFoundOnFs := fsHashesOnDisk[dbInfo.Hash]; contentFoundOnFs {
				result.ProposedMoves = append(result.ProposedMoves, types.MoveInfo{
					OldPath:     dbPath,
					NewLocation: newLocation,
				})
				handledFsPaths[newLocation.Path] = true
				delete(fsHashesOnDisk, dbInfo.Hash) // Prevent re-use for another move
			} else {
				// Content not found anywhere in scanned dirs. Propose deletion.
				var tags []string
				if dbInfo.TagsCache != "" {
					tags = strings.Split(dbInfo.TagsCache, " ")
				}
				result.ProposedDeletes = append(result.ProposedDeletes, types.FileInfo{
					Path: dbPath, Size: dbInfo.Size, Tags: tags,
				})
			}
		}
	}

	// --- Phase 3: Find Moves-In and New Duplicates ---
	unhandledHashes := make([]string, 0)
	unhandledFsLocations := make(map[string]types.LocationInfo)
	for fsPath, fsInfo := range fsLocations {
		if !handledFsPaths[fsPath] {
			unhandledHashes = append(unhandledHashes, fsInfo.Hash)
			unhandledFsLocations[fsInfo.Hash] = fsInfo
		}
	}

	if len(unhandledHashes) > 0 {
		allOldPaths, err := c.store.BatchGetPathsForHashes(c.store, unhandledHashes)
		if err != nil {
			return result, fmt.Errorf("could not look up old paths for found content: %w", err)
		}

		for hash, fsInfo := range unhandledFsLocations {
			oldPaths, contentWasKnown := allOldPaths[hash]
			isMove := false
			if contentWasKnown {
				for _, oldPath := range oldPaths {
					if _, ok := fsLocations[oldPath]; ok {
						continue // Don't check against a path that's also in the current scan scope.
					}
					// Check if the old location is now empty. This is the key check for a move.
					if _, err := os.Stat(oldPath); os.IsNotExist(err) {
						result.ProposedMoves = append(result.ProposedMoves, types.MoveInfo{
							OldPath:     oldPath,
							NewLocation: fsInfo,
						})
						isMove = true
						break // Found one missing old path, that's enough to classify it as a move.
					}
				}
			}
			if !isMove {
				result.ProposedAdds = append(result.ProposedAdds, fsInfo)
			}
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
func (c *Client) RehashFiles(filePaths []string, progressCb func(path string, status types.RehashStatus, err error), useMetadataHeuristic bool) {
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

		// Path 1: Fast exit using heuristic if requested and metadata matches.
		if useMetadataHeuristic && (fsInfo.Size() == dbInfo.Size && fsInfo.ModTime().Unix() == dbInfo.ModTime) {
			progressCb(originalPath, types.StatusSkippedUnchanged, nil)
			continue
		}

		// Path 2: Heuristic was false OR failed. We must verify by hashing.
		newHash, err := c.hasher.HashFile(absPath)
		if err != nil {
			progressCb(originalPath, 0, fmt.Errorf("hashing failed: %w", err))
			continue
		}

		// Case A: Content is identical.
		if newHash == dbInfo.Hash {
			// Check if only metadata changed.
			if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
				err := c.store.UpdateLocationMetadata(absPath, fsInfo.Size(), fsInfo.ModTime().Unix())
				if err != nil {
					progressCb(originalPath, 0, fmt.Errorf("metadata update failed: %w", err))
				} else {
					progressCb(originalPath, types.StatusMetadataUpdated, nil)
				}
			} else {
				// Hashes and metadata match, truly unchanged.
				progressCb(originalPath, types.StatusSkippedUnchanged, nil)
			}
			continue
		}

		// Case B: Content has definitively changed. Proceed with full rehash.
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
