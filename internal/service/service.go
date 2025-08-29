package service

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/gooru/internal/database"
	"gooru.local/gooru/internal/query"
	"gooru.local/gooru/internal/types"
)

// Service encapsulates the core business logic.
type Service struct {
	Store *database.Store
}

// resolvePath canonicalizes a path. If it doesn't exist, it returns the
// absolute path of the input to allow for clean "file not found" errors later.
func resolvePath(filePath string) (string, error) {
	// If a file doesn't exist, os.Lstat is the first to tell us. We don't
	// try to resolve symlinks in this case, just return its absolute path.
	if _, err := os.Lstat(filePath); os.IsNotExist(err) {
		return filepath.Abs(filePath)
	}

	return filepath.Abs(filePath)
}

// NewService creates a new Service.
func NewService(store *database.Store) *Service {
	return &Service{Store: store}
}

// TagFiles adds tags to multiple files using a high-performance batching strategy.
func (s *Service) TagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	return s.tagOperation(filePaths, tags, progressCb, false)
}

// UntagFiles untags multiple files with the given tags, with safety checks and intelligent move detection.
func (s *Service) UntagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	if len(tags) == 0 {
		for _, fp := range filePaths {
			progressCb(fp, nil)
		}
		return nil
	}

	// Phase 1: Pre-process, hash where necessary, and detect moves.
	absPaths := make([]string, 0, len(filePaths))
	originalPathMap := make(map[string]string, len(filePaths))
	for _, fp := range filePaths {
		absPath, err := resolvePath(fp)
		if err != nil {
			progressCb(fp, err)
			continue
		}
		absPaths = append(absPaths, absPath)
		originalPathMap[absPath] = fp
	}

	dbLocations, err := s.Store.BatchGetLocationsByPaths(absPaths)
	if err != nil {
		return fmt.Errorf("failed to look up file locations: %w", err)
	}

	// Find files not in the DB by path, which might have been moved.
	filesToHash := make([]string, 0)
	for _, absPath := range absPaths {
		if _, exists := dbLocations[absPath]; !exists {
			// Check if file exists on disk before attempting to hash it.
			if _, err := os.Stat(absPath); err == nil {
				filesToHash = append(filesToHash, absPath)
			}
		}
	}

	// If there are potential moves, reconcile them first.
	if len(filesToHash) > 0 {
		hashResults := concurrentlyHashFiles(filesToHash)
		potentialMoves := make(map[string]string)
		for _, res := range hashResults {
			if res.err == nil {
				potentialMoves[res.hash] = res.filePath
			}
		}

		if err := s.reconcileMoves(potentialMoves); err != nil {
			return fmt.Errorf("failed to reconcile moved files: %w", err)
		}

		// Re-fetch location info for the files that were just moved.
		newlyFoundLocations, err := s.Store.BatchGetLocationsByPaths(filesToHash)
		if err != nil {
			return fmt.Errorf("failed to re-fetch reconciled locations: %w", err)
		}
		for path, loc := range newlyFoundLocations {
			dbLocations[path] = loc
		}
	}

	// Phase 2: Perform the untag operation on all valid files.
	validHashes := make(map[string]struct{})
	processedPaths := make(map[string]bool)

	for _, absPath := range absPaths {
		originalPath := originalPathMap[absPath]
		fsInfo, err := os.Stat(absPath)
		if err != nil {
			if !processedPaths[originalPath] {
				progressCb(originalPath, err)
				processedPaths[originalPath] = true
			}
			continue
		}

		dbInfo, existsInDb := dbLocations[absPath]
		if !existsInDb {
			if !processedPaths[originalPath] {
				progressCb(originalPath, fmt.Errorf("file not found in database"))
				processedPaths[originalPath] = true
			}
			continue
		}

		// Critical safety check: only untag if file content is what we expect.
		if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
			if !processedPaths[originalPath] {
				progressCb(originalPath, fmt.Errorf("file has been modified; please re-tag it first"))
				processedPaths[originalPath] = true
			}
			continue
		}

		validHashes[dbInfo.Hash] = struct{}{}
	}

	if len(validHashes) == 0 {
		for _, originalPath := range filePaths {
			if !processedPaths[originalPath] {
				progressCb(originalPath, nil)
			}
		}
		return nil
	}

	// Phase 3: Transactional untagging.
	tx, err := s.Store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	tagIDMap, err := s.Store.BatchGetOrCreateTags(tx, parsedTags)
	if err != nil {
		return fmt.Errorf("failed to look up tags: %w", err)
	}

	pairsToDisassociate := make([]database.ContentTagPair, 0, len(validHashes)*len(tags))
	for hash := range validHashes {
		for _, tagStr := range tags {
			if tagID, ok := tagIDMap[tagStr]; ok {
				pairsToDisassociate = append(pairsToDisassociate, database.ContentTagPair{
					ContentHash: hash,
					TagID:       tagID,
				})
			}
		}
	}

	if err := s.Store.BatchDisassociateTags(tx, pairsToDisassociate); err != nil {
		return fmt.Errorf("failed to batch disassociate tags: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, originalPath := range filePaths {
		if !processedPaths[originalPath] {
			progressCb(originalPath, nil)
		}
	}

	return nil
}

// SetTagsForFiles sets the tags for multiple files, replacing any existing ones, using a batching strategy.
func (s *Service) SetTagsForFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	return s.tagOperation(filePaths, tags, progressCb, true)
}

// tagOperation is the shared, high-performance batching logic for tag and settags.
func (s *Service) tagOperation(filePaths []string, tags []string, progressCb func(filePath string, err error), isSet bool) error {
	// Phase 1: Collect info from FS and DB, and hash necessary files.
	// 1a. Resolve paths and collect absolute paths.
	absPaths := make([]string, 0, len(filePaths))
	originalPathMap := make(map[string]string, len(filePaths))
	for _, fp := range filePaths {
		absPath, err := resolvePath(fp)
		if err != nil {
			progressCb(fp, err)
			continue
		}
		absPaths = append(absPaths, absPath)
		originalPathMap[absPath] = fp
	}

	// 1b. Get existing location data from the DB.
	dbLocations, err := s.Store.BatchGetLocationsByPaths(absPaths)
	if err != nil {
		return fmt.Errorf("could not get existing file data: %w", err)
	}

	// 1c. Determine which files actually need to be hashed.
	filesToHash := make([]string, 0)
	for _, absPath := range absPaths {
		info, err := os.Stat(absPath)
		if err != nil {
			progressCb(originalPathMap[absPath], err)
			continue
		}
		dbInfo, existsInDb := dbLocations[absPath]
		if existsInDb && info.Size() == dbInfo.Size && info.ModTime().Unix() == dbInfo.ModTime {
			continue
		}
		filesToHash = append(filesToHash, absPath)
	}

	// 1d. Concurrently hash all the necessary files.
	hashResults := concurrentlyHashFiles(filesToHash)

	// Phase 2: Prepare data structures for transaction, identifying potential moves.
	type fileData struct {
		path string // original path for callbacks
		info types.LocationInfo
	}
	allFileData := make([]fileData, 0, len(filePaths))
	allHashes := make([]string, 0, len(filePaths))
	locationsToUpsert := make(map[string]types.LocationInfo)
	potentialMoves := make(map[string]string) // hash -> newPath
	processedPaths := make(map[string]bool)

	for _, absPath := range absPaths {
		originalPath := originalPathMap[absPath]
		info, err := os.Stat(absPath)
		if err != nil {
			if !processedPaths[originalPath] {
				progressCb(originalPath, err)
				processedPaths[originalPath] = true
			}
			continue
		}

		var hash string
		dbInfo, existsInDb := dbLocations[absPath]
		wasHashed := false

		if existsInDb && info.Size() == dbInfo.Size && info.ModTime().Unix() == dbInfo.ModTime {
			hash = dbInfo.Hash
		} else {
			result, ok := hashResults[absPath]
			if !ok || result.err != nil {
				if !processedPaths[originalPath] {
					errMsg := "file processing failed"
					if result.err != nil {
						errMsg = fmt.Sprintf("hashing failed: %v", result.err)
					}
					progressCb(originalPath, fmt.Errorf("%s", errMsg))
					processedPaths[originalPath] = true
				}
				continue
			}
			hash = result.hash
			wasHashed = true
		}

		if wasHashed {
			potentialMoves[hash] = absPath
		}

		locInfo := types.LocationInfo{
			Path:      absPath,
			Hash:      hash,
			Size:      info.Size(),
			ModTime:   info.ModTime().Unix(),
			Extension: filepath.Ext(absPath),
		}
		allFileData = append(allFileData, fileData{path: originalPath, info: locInfo})
		allHashes = append(allHashes, hash)
		locationsToUpsert[absPath] = locInfo
	}

	if len(allFileData) == 0 {
		return nil // No files could be processed.
	}

	// Phase 3: The Transaction.
	tx, err := s.Store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Phase 3a: Intelligently handle file moves.
	if len(potentialMoves) > 0 {
		hashesToCheck := make([]string, 0, len(potentialMoves))
		for hash := range potentialMoves {
			hashesToCheck = append(hashesToCheck, hash)
		}

		hashToOldPaths, err := s.Store.BatchGetPathsForHashes(tx, hashesToCheck)
		if err != nil {
			return fmt.Errorf("failed to check for existing content paths: %w", err)
		}

		for hash, oldPaths := range hashToOldPaths {
			newPath := potentialMoves[hash]
			for _, oldPath := range oldPaths {
				// If old path is the same as new path, it can't be a move.
				if oldPath == newPath {
					continue
				}
				if _, statErr := os.Stat(oldPath); os.IsNotExist(statErr) {
					// Confirmed move! The old path is gone.
					if err := s.Store.UpdateLocationPath(tx, oldPath, newPath); err != nil {
						return fmt.Errorf("failed to update moved path from '%s' to '%s': %w", oldPath, newPath, err)
					}
					// This was a move, so don't also treat it as a new location to insert.
					delete(locationsToUpsert, newPath)
					// We've handled the move for this content, stop checking its other old paths.
					break
				}
			}
		}
	}

	// Phase 3b: Batch database operations for all remaining files.
	if err := s.Store.BatchInsertContents(tx, allHashes); err != nil {
		return fmt.Errorf("failed to batch insert contents: %w", err)
	}
	if err := s.Store.BatchUpsertLocations(tx, locationsToUpsert); err != nil {
		return fmt.Errorf("failed to batch upsert locations: %w", err)
	}
	if isSet {
		if err := s.Store.BatchClearTagsForContent(tx, allHashes); err != nil {
			return fmt.Errorf("failed to batch clear tags: %w", err)
		}
	}
	if len(tags) > 0 {
		// Get/create tags *after* clearing, so we don't fetch IDs that might get deleted by the cleanup trigger.
		parsedTags := make([]types.ParsedTag, len(tags))
		for i, t := range tags {
			parsedTags[i] = query.ParseTag(t)
		}
		tagIDMap, err := s.Store.BatchGetOrCreateTags(tx, parsedTags)
		if err != nil {
			return fmt.Errorf("failed to get or create tags: %w", err)
		}

		pairs := make([]database.ContentTagPair, 0, len(allHashes)*len(tags))
		for _, hash := range allHashes {
			for _, tagStr := range tags {
				pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagIDMap[tagStr]})
			}
		}
		if err := s.Store.BatchAssociateTags(tx, pairs); err != nil {
			return fmt.Errorf("failed to batch associate tags: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, data := range allFileData {
		progressCb(data.path, nil)
	}

	return nil
}

// GetTagsForFile retrieves all tags for a given file, with a safety check.
func (s *Service) GetTagsForFile(filePath string) ([]string, error) {
	absPath, err := resolvePath(filePath)
	if err != nil {
		return nil, err
	}

	// Safety Check
	fsInfo, err := os.Stat(absPath)
	if err != nil {
		// If file doesn't exist on disk, it can't have tags.
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	dbInfo, err := s.Store.GetLocationByPath(absPath)
	if err != nil {
		// If not in DB, it has no tags.
		if err == sql.ErrNoRows {
			return []string{}, nil
		}
		return nil, fmt.Errorf("database lookup failed: %w", err)
	}

	// If metadata doesn't match, treat as not in DB.
	if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
		return []string{}, nil
	}

	return s.Store.GetTagsForContent(dbInfo.Hash)
}

// ListAllFiles lists all files known to the system.
func (s *Service) ListAllFiles() ([]string, error) {
	return s.Store.ListAllFiles()
}

// ListFilesByTag lists all files associated with a given tag.
func (s *Service) ListFilesByTag(tag string) ([]string, error) {
	parsedTag := query.ParseTag(tag)
	return s.Store.ListFilesByTag(parsedTag.Key, parsedTag.Value)
}

// ListFilesByTagsAnd lists all files associated with a given set of tags (AND query).
func (s *Service) ListFilesByTagsAnd(tags []string) ([]string, error) {
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	return s.Store.ListFilesByTagsAnd(parsedTags)
}

// ListFilesByQuery parses and executes a complex query expression.
func (s *Service) ListFilesByQuery(expression string, verbose bool) ([]string, error) {
	if strings.TrimSpace(expression) == "" {
		return []string{}, nil
	}
	ast, err := query.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("could not parse query: %w", err)
	}

	sqlQuery, args := query.Build(ast)
	if sqlQuery == "" {
		return []string{}, nil
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\n")
		fmt.Fprintf(os.Stderr, "Expression: %s\n", expression)
		fmt.Fprintf(os.Stderr, "Built SQL : %s\n", sqlQuery)
		fmt.Fprintf(os.Stderr, "SQL Args  : %v\n", args)
		fmt.Fprintf(os.Stderr, "-------------\n")
	}

	return s.Store.GetPathsByContentQuery(sqlQuery, args)
}

// GetAllFilesInfo gets detailed info for all files known to the system.
func (s *Service) GetAllFilesInfo() ([]types.FileInfo, error) {
	return s.Store.GetAllFilesInfo()
}

// GetFilesInfoByTag gets detailed info for all files associated with a given tag.
func (s *Service) GetFilesInfoByTag(tag string) ([]types.FileInfo, error) {
	parsedTag := query.ParseTag(tag)
	return s.Store.GetFilesInfoByTag(parsedTag.Key, parsedTag.Value)
}

// GetFilesInfoByTagsAnd gets detailed info for all files associated with a given set of tags (AND query).
func (s *Service) GetFilesInfoByTagsAnd(tags []string) ([]types.FileInfo, error) {
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	return s.Store.GetFilesInfoByTagsAnd(parsedTags)
}

// GetFilesInfoByQuery parses and executes a complex query expression, returning full file info.
func (s *Service) GetFilesInfoByQuery(expression string, verbose bool) ([]types.FileInfo, error) {
	if strings.TrimSpace(expression) == "" {
		return []types.FileInfo{}, nil
	}
	ast, err := query.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("could not parse query: %w", err)
	}

	sqlQuery, args := query.Build(ast)
	if sqlQuery == "" {
		return []types.FileInfo{}, nil
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\n")
		fmt.Fprintf(os.Stderr, "Expression: %s\n", expression)
		fmt.Fprintf(os.Stderr, "Built SQL : %s\n", sqlQuery)
		fmt.Fprintf(os.Stderr, "SQL Args  : %v\n", args)
		fmt.Fprintf(os.Stderr, "-------------\n")
	}

	return s.Store.GetFilesInfoByContentQuery(sqlQuery, args)
}

// GetAllTags retrieves all tags from the database.
func (s *Service) GetAllTags() ([]string, error) {
	return s.Store.GetAllTags()
}

// reconcileMoves checks for and atomically updates the paths of moved files.
func (s *Service) reconcileMoves(potentialMoves map[string]string) error {
	if len(potentialMoves) == 0 {
		return nil
	}

	tx, err := s.Store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	hashesToCheck := make([]string, 0, len(potentialMoves))
	for hash := range potentialMoves {
		hashesToCheck = append(hashesToCheck, hash)
	}

	hashToOldPaths, err := s.Store.BatchGetPathsForHashes(tx, hashesToCheck)
	if err != nil {
		return fmt.Errorf("failed to check for existing content paths: %w", err)
	}

	for hash, oldPaths := range hashToOldPaths {
		newPath := potentialMoves[hash]
		for _, oldPath := range oldPaths {
			if oldPath == newPath {
				continue
			}
			if _, statErr := os.Stat(oldPath); os.IsNotExist(statErr) {
				if err := s.Store.UpdateLocationPath(tx, oldPath, newPath); err != nil {
					return fmt.Errorf("failed to update moved path from '%s' to '%s': %w", oldPath, newPath, err)
				}
				break // Handled move for this content hash.
			}
		}
	}

	return tx.Commit()
}

// EditPath manually updates a file's path in the database.
func (s *Service) EditPath(oldPath, newPath string) error {
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

	return s.Store.UpdatePath(absOldPath, absNewPath, oldPath, newPath)
}

// NeedsRelink performs a fast check using filesystem metadata to see if a relink is necessary.
func (s *Service) NeedsRelink(dirs []string) (bool, error) {
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return false, err
	}
	dbLocations, err := s.Store.GetLocationsForDirs(absDirs)
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
func (s *Service) Relink(dirs []string) (types.RelinkResult, error) {
	result := types.RelinkResult{}
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return result, err
	}

	// 1. Get initial state from the database.
	dbLocations, err := s.Store.GetLocationsForDirs(absDirs)
	if err != nil {
		return result, fmt.Errorf("could not get db locations: %w", err)
	}
	sizeToHashes, err := s.Store.GetSizeToHashesMap()
	if err != nil {
		return result, fmt.Errorf("could not build size-to-hash map: %w", err)
	}

	// 2. Perform the intelligent, targeted filesystem scan.
	fsLocations, filesScanned := s.scanDirsConcurrently(absDirs, sizeToHashes)
	result.Stats.FilesScanned = filesScanned

	// 3. Reconcile states.
	handledDbPaths := make(map[string]bool)
	fsHashToPaths := make(map[string][]string)
	for path, info := range fsLocations {
		fsHashToPaths[info.Hash] = append(fsHashToPaths[info.Hash], path)
	}

	// 3a. Find moves and unchanged files.
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

				result.ProposedMoves = append(result.ProposedMoves, types.MoveInfo{
					OldPath: dbPath,
					NewPath: newPath,
					Size:    dbInfo.Size,
					Tags:    dbInfo.TagsCache,
				})
				handledDbPaths[dbPath] = true
				delete(fsLocations, newPath)       // This fs location is accounted for
				fsHashToPaths[dbInfo.Hash][i] = "" // Mark this path as used
				goto nextDbPath                    // Move to the next db path
			}
		}
	nextDbPath:
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
func (s *Service) ApplyRelinkChanges(changes types.RelinkResult) (types.RelinkStats, error) {
	stats := types.RelinkStats{}
	if len(changes.ProposedMoves) == 0 && len(changes.ProposedAdds) == 0 && len(changes.ProposedDeletes) == 0 {
		return stats, nil
	}

	tx, err := s.Store.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	// 1. Apply moves
	for _, move := range changes.ProposedMoves {
		if err := s.Store.UpdateLocationPath(tx, move.OldPath, move.NewPath); err != nil {
			return stats, fmt.Errorf("failed to update moved path from '%s' to '%s': %w", move.OldPath, move.NewPath, err)
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

		addedCount, err := s.Store.ApplyRelinkAdditionsTx(tx, toAdd)
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
		removedCount, err := s.Store.RemoveLocationsByPathTx(tx, pathsToDelete)
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
func (s *Service) PruneLocations(paths []string) (int, error) {
	return s.Store.RemoveLocationsByPath(paths)
}

func toAbsolutePaths(paths []string) ([]string, error) {
	absPaths := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		absPaths[i] = abs
	}
	return absPaths, nil
}
