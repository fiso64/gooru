package gooru

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gooru.local/internal/relink"
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

	absNewPath, err := resolvePath(newPath)
	if err != nil {
		return fmt.Errorf("could not resolve new path '%s': %w", newPath, err)
	}

	// Managed uploads keep a canonical logical path in locations while their
	// bytes live at a separate physical storage path. Renaming that logical
	// identity must inspect the stored source rather than requiring the new
	// logical path to exist on disk. Ordinary locations still inspect newPath,
	// which is the file the user moved/renamed before invoking editpath.
	sourcePath := absNewPath
	tracked, err := c.store.GetLocationSourceByPath(absOldPath)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("could not inspect old tracked path '%s': %w", oldPath, err)
	}
	if err == nil && tracked.StoragePath != "" {
		sourcePath = tracked.StoragePath
	}

	logicalInfo, err := c.hasher.FileMetadata(sourcePath)
	if err != nil {
		return fmt.Errorf("could not inspect new path '%s': %w", newPath, err)
	}

	newInfo := types.LocationInfo{
		Path:      absNewPath,
		Size:      logicalInfo.Size,
		ModTime:   logicalInfo.ModTime.Unix(),
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
	trackedSources, err := c.store.BatchGetLocationSourcesByPaths(locationPaths(dbLocations))
	if err != nil {
		return false, fmt.Errorf("could not look up tracked sources for pre-check: %w", err)
	}

	// IMPORTANT: Get the size-to-hash map to filter the FS walk, exactly like the full Relink scan does.
	// This ensures both functions see the same set of "relevant" files on disk.
	sizeToHashes, err := c.store.GetSizeToHashesMap()
	if err != nil {
		return false, fmt.Errorf("could not build size-to-hash map for pre-check: %w", err)
	}

	// Get FS state for "relevant" files in the given directories. Logical
	// metadata is required here: encrypted containers have a different physical
	// size from the plaintext identity stored in the database. Cache it during
	// the walk so the common metadata-only path opens each source only once.
	type logicalMetadata struct {
		size    int64
		modTime int64
	}
	fsPaths := make(map[string]logicalMetadata)
	for _, dir := range absDirs {
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				info, err := c.hasher.FileMetadata(path)
				if err != nil {
					return fmt.Errorf("inspect %q: %w", path, err)
				}
				// The core filtering logic that must match Relink's scanner.
				if _, ok := sizeToHashes[info.Size]; ok {
					fsPaths[path] = logicalMetadata{size: info.Size, modTime: info.ModTime.Unix()}
				}
			}
			return nil
		})
		if walkErr != nil {
			return false, fmt.Errorf("failed to walk directory %s: %w", dir, walkErr)
		}
	}

	// A physical managed-storage alias is already represented by its canonical
	// logical location. Hide aliases found by the filesystem walk even when that
	// logical location is outside the directories being checked; otherwise the
	// backing object looks like an extra ordinary file forever.
	scannedPaths := make([]string, 0, len(fsPaths))
	for path := range fsPaths {
		scannedPaths = append(scannedPaths, path)
	}
	managedAliases, err := c.store.BatchGetManagedLocationSourcesByPhysicalPaths(scannedPaths)
	if err != nil {
		return false, fmt.Errorf("could not look up managed backing aliases for pre-check: %w", err)
	}
	for physicalPath := range managedAliases {
		delete(fsPaths, physicalPath)
	}

	// Managed locations use their logical path as database identity but keep the
	// bytes at StoragePath. Add their logical identity to the comparison only
	// when the physical source can be inspected through the configured policy.
	for path, source := range trackedSources {
		if source.StoragePath == "" {
			continue
		}
		info, err := c.hasher.FileMetadata(source.StoragePath)
		if err != nil {
			return false, fmt.Errorf("inspect managed source %q: %w", source.StoragePath, err)
		}
		delete(fsPaths, source.StoragePath)
		fsPaths[path] = logicalMetadata{size: info.Size, modTime: info.ModTime.Unix()}
	}

	// Compare every tracked path first. Missing or changed tracked content always
	// requires a full relink regardless of unrelated files elsewhere in the scan.
	for path, dbInfo := range dbLocations {
		logicalInfo, ok := fsPaths[path]
		if !ok {
			return true, nil // A DB path is missing from the filtered filesystem set.
		}

		if alwaysVerifyHash {
			sourcePath := path
			if source, ok := trackedSources[path]; ok && source.StoragePath != "" {
				sourcePath = source.StoragePath
			}
			currentHash, err := c.hasher.HashFile(sourcePath)
			if err != nil {
				return false, fmt.Errorf("hash %q during relink pre-check: %w", sourcePath, err)
			}
			if currentHash != dbInfo.Hash {
				return true, nil // Content hash mismatch.
			}
		} else if logicalInfo.size != dbInfo.Size || logicalInfo.modTime != dbInfo.ModTime {
			return true, nil // Metadata mismatch.
		}
	}

	// The size filter is deliberately broader than the full planner: unrelated
	// content can share a byte size with known content. Hash only those extra
	// same-size paths so the pre-check agrees with Relink without paying the
	// full-scan hashing cost for the common no-extra-files case.
	for path, logicalInfo := range fsPaths {
		if _, tracked := dbLocations[path]; tracked {
			continue
		}
		currentHash, err := c.hasher.HashFile(path)
		if err != nil {
			return false, fmt.Errorf("hash %q during relink pre-check: %w", path, err)
		}
		for _, knownHash := range sizeToHashes[logicalInfo.size] {
			if currentHash == knownHash {
				return true, nil
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

	dbLocationsInScope, err := c.store.GetLocationsForDirs(absDirs)
	if err != nil {
		return result, fmt.Errorf("could not get in-scope db locations: %w", err)
	}
	sizeToHashes, err := c.store.GetSizeToHashesMap()
	if err != nil {
		return result, fmt.Errorf("could not build size-to-hash map: %w", err)
	}
	fsLocations, filesScanned, err := scanning.DirsConcurrently(absDirs, sizeToHashes, c.hasher)
	if err != nil {
		return result, fmt.Errorf("scan relink directories: %w", err)
	}

	managedAliases, err := c.store.BatchGetManagedLocationSourcesByPhysicalPaths(locationPaths(fsLocations))
	if err != nil {
		return result, fmt.Errorf("could not look up managed backing aliases: %w", err)
	}
	for physicalPath := range managedAliases {
		delete(fsLocations, physicalPath)
	}

	fsHashes := uniqueHashes(fsLocations)
	knownPathsByHash, err := c.store.BatchGetPathsForHashes(c.store, fsHashes)
	if err != nil {
		return result, fmt.Errorf("could not look up old paths for found content: %w", err)
	}
	trackedSources, err := c.store.BatchGetLocationSourcesByPaths(relinkSourcePaths(knownPathsByHash, dbLocationsInScope))
	if err != nil {
		return result, fmt.Errorf("could not look up tracked sources for found content: %w", err)
	}

	// A managed location whose physical source still exists is not missing merely
	// because its canonical logical path is intentionally absent. Synthesize that
	// logical identity into the planner's filesystem view so the first pass cannot
	// propose a destructive move/delete for a live managed upload. If the backing
	// path is itself inside a scanned directory, remove that physical alias first
	// so it cannot also be proposed as a separate ordinary location.
	for path, dbInfo := range dbLocationsInScope {
		source, ok := trackedSources[path]
		if !ok || source.StoragePath == "" {
			continue
		}
		if _, err := os.Stat(source.StoragePath); err == nil || !os.IsNotExist(err) {
			delete(fsLocations, source.StoragePath)
			fsLocations[path] = dbInfo
		}
	}

	missingKnownPath := missingPaths(knownPathsByHash, trackedSources)
	return relink.Plan(relink.PlanInput{
		DBLocations:      dbLocationsInScope,
		FSLocations:      fsLocations,
		KnownPathsByHash: knownPathsByHash,
		MissingKnownPath: missingKnownPath,
		FilesScanned:     filesScanned,
	}), nil
}

func uniqueHashes(locations map[string]types.LocationInfo) []string {
	seen := make(map[string]struct{})
	hashes := make([]string, 0, len(locations))
	for _, loc := range locations {
		if _, ok := seen[loc.Hash]; ok {
			continue
		}
		seen[loc.Hash] = struct{}{}
		hashes = append(hashes, loc.Hash)
	}
	return hashes
}

func locationPaths(locations map[string]types.LocationInfo) []string {
	paths := make([]string, 0, len(locations))
	for path := range locations {
		paths = append(paths, path)
	}
	return paths
}

func uniqueKnownPaths(pathsByHash map[string][]string) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	for _, knownPaths := range pathsByHash {
		for _, path := range knownPaths {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}

func relinkSourcePaths(pathsByHash map[string][]string, dbLocations map[string]types.LocationInfo) []string {
	paths := uniqueKnownPaths(pathsByHash)
	seen := make(map[string]struct{}, len(paths)+len(dbLocations))
	for _, path := range paths {
		seen[path] = struct{}{}
	}
	for path := range dbLocations {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func missingPaths(pathsByHash map[string][]string, sourcesByPath map[string]types.LocationInfo) map[string]bool {
	missing := make(map[string]bool)
	for _, paths := range pathsByHash {
		for _, path := range paths {
			if _, checked := missing[path]; checked {
				continue
			}
			sourcePath := path
			if source, ok := sourcesByPath[path]; ok && source.StoragePath != "" {
				sourcePath = source.StoragePath
			}
			_, err := os.Stat(sourcePath)
			missing[path] = os.IsNotExist(err)
		}
	}
	return missing
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

	deletePaths := relinkDeletePaths(changes.ProposedDeletes)
	preDeletePaths := relinkTargetDeletePaths(changes, deletePaths)
	removed, err := c.removeRelinkPathsTx(tx, preDeletePaths)
	if err != nil {
		return stats, err
	}
	stats.LocationsRemoved += removed

	for _, move := range changes.ProposedMoves {
		if err := c.store.UpdateRelinkedLocation(tx, move.OldPath, move.NewLocation); err != nil {
			return stats, fmt.Errorf("failed to update moved path from '%s' to '%s': %w", move.OldPath, move.NewLocation.Path, err)
		}
	}
	stats.LocationsAdded += len(changes.ProposedMoves)

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

	remainingDeletePaths := subtractPaths(deletePaths, preDeletePaths)
	removed, err = c.removeRelinkPathsTx(tx, remainingDeletePaths)
	if err != nil {
		return stats, err
	}
	stats.LocationsRemoved += removed

	if err := tx.Commit(); err != nil {
		return stats, err
	}

	return stats, nil
}

func (c *Client) removeRelinkPathsTx(tx interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}, paths []string) (int, error) {
	removedCount, err := c.store.RemoveLocationsByPathTx(tx, paths)
	if err != nil {
		return 0, fmt.Errorf("failed to apply deletions: %w", err)
	}
	return removedCount, nil
}

func relinkDeletePaths(deletes []types.FileInfo) []string {
	paths := make([]string, 0, len(deletes))
	seen := make(map[string]bool)
	for _, del := range deletes {
		if seen[del.Path] {
			continue
		}
		seen[del.Path] = true
		paths = append(paths, del.Path)
	}
	return paths
}

func relinkTargetDeletePaths(changes types.RelinkResult, deletePaths []string) []string {
	deleteSet := make(map[string]bool, len(deletePaths))
	for _, path := range deletePaths {
		deleteSet[path] = true
	}

	targets := make(map[string]bool)
	for _, move := range changes.ProposedMoves {
		targets[move.NewLocation.Path] = true
	}
	for _, add := range changes.ProposedAdds {
		targets[add.Path] = true
	}

	paths := make([]string, 0)
	for _, path := range deletePaths {
		if deleteSet[path] && targets[path] {
			paths = append(paths, path)
		}
	}
	return paths
}

func subtractPaths(paths, remove []string) []string {
	removeSet := make(map[string]bool, len(remove))
	for _, path := range remove {
		removeSet[path] = true
	}

	remaining := make([]string, 0, len(paths))
	for _, path := range paths {
		if !removeSet[path] {
			remaining = append(remaining, path)
		}
	}
	return remaining
}

// PruneLocations removes a list of file paths from the database.
func (c *Client) PruneLocations(paths []string) (int, error) {
	return c.store.RemoveLocationsByPath(paths)
}

// RehashFiles updates content for tracked files while preserving tags.
// It is kept for API compatibility; RehashTrackedFiles is the single source of
// rehash behavior so managed-storage and ordinary locations cannot drift apart.
func (c *Client) RehashFiles(filePaths []string, progressCb func(path string, status types.RehashStatus, err error), useMetadataHeuristic bool) {
	c.RehashTrackedFiles(filePaths, progressCb, useMetadataHeuristic)
}
