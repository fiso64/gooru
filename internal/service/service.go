package service

import (
	"fmt"
	"os"
	"path/filepath"

	"gooru.local/gooru/internal/database"
	"gooru.local/gooru/internal/hashing"
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

// UntagFiles untags multiple files with the given tags using a high-performance batching strategy.
func (s *Service) UntagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	if len(tags) == 0 {
		// Nothing to do, report success for all files found.
		for _, fp := range filePaths {
			progressCb(fp, nil)
		}
		return nil
	}

	// 1. Pre-process to gather file paths.
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

	// 2. Fetch data from DB in batches *before* the transaction.
	pathHash, err := s.Store.BatchFindContentHashesByPaths(absPaths)
	if err != nil {
		return fmt.Errorf("failed to look up file hashes: %w", err)
	}

	// 3. Start transaction for the write operations.
	tx, err := s.Store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 3a. Get all necessary tag IDs.
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	// We use BatchGetOrCreateTags which safely handles non-existent tags.
	// We only care about the returned map of existing/newly created tags.
	tagIDMap, err := s.Store.BatchGetOrCreateTags(tx, parsedTags)
	if err != nil {
		return fmt.Errorf("failed to look up tags: %w", err)
	}

	// 3b. Prepare the batch disassociation.
	pairsToDisassociate := make([]database.ContentTagPair, 0, len(pathHash)*len(tags))
	for _, hash := range pathHash {
		for _, tagStr := range tags {
			if tagID, ok := tagIDMap[tagStr]; ok {
				pairsToDisassociate = append(pairsToDisassociate, database.ContentTagPair{
					ContentHash: hash,
					TagID:       tagID,
				})
			}
		}
	}

	// 3c. Execute the batch delete.
	if err := s.Store.BatchDisassociateTags(tx, pairsToDisassociate); err != nil {
		return fmt.Errorf("failed to batch disassociate tags: %w", err)
	}

	// 4. Commit.
	if err := tx.Commit(); err != nil {
		return err
	}

	// 5. Report success/failure via callback.
	for _, absPath := range absPaths {
		originalPath := originalPathMap[absPath]
		if _, ok := pathHash[absPath]; !ok {
			progressCb(originalPath, fmt.Errorf("file not found in database"))
		} else {
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
	// 1. Pre-process all files to gather data before starting the transaction.
	type fileData struct {
		path string // original path for callbacks
		info types.LocationInfo
	}
	allFileData := make([]fileData, 0, len(filePaths))
	allHashes := make([]string, 0, len(filePaths))
	locationsToUpsert := make(map[string]types.LocationInfo, len(filePaths))

	for _, filePath := range filePaths {
		absPath, err := resolvePath(filePath)
		if err != nil {
			progressCb(filePath, err)
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			progressCb(filePath, err)
			continue
		}
		hash, err := hashing.HashFile(absPath)
		if err != nil {
			progressCb(filePath, err)
			continue
		}
		locInfo := types.LocationInfo{
			Path:      absPath, // For BatchUpsertLocations
			Hash:      hash,
			Size:      info.Size(),
			ModTime:   info.ModTime().Unix(),
			Extension: filepath.Ext(absPath),
		}
		allFileData = append(allFileData, fileData{path: filePath, info: locInfo})
		allHashes = append(allHashes, hash)
		locationsToUpsert[absPath] = locInfo
	}

	// 2. Start transaction and perform all DB operations in batches.
	tx, err := s.Store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 2a. Get or create all necessary tags.
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	tagIDMap, err := s.Store.BatchGetOrCreateTags(tx, parsedTags)
	if err != nil {
		return fmt.Errorf("failed to get or create tags: %w", err)
	}

	// 2b. Ensure all content hashes exist.
	if err := s.Store.BatchInsertContents(tx, allHashes); err != nil {
		return fmt.Errorf("failed to batch insert contents: %w", err)
	}

	// 2c. Insert or update all file locations.
	if err := s.Store.BatchUpsertLocations(tx, locationsToUpsert); err != nil {
		return fmt.Errorf("failed to batch upsert locations: %w", err)
	}

	// 2d. (For settags) Clear all previous tag associations for the files.
	if isSet {
		if err := s.Store.BatchClearTagsForContent(tx, allHashes); err != nil {
			return fmt.Errorf("failed to batch clear tags: %w", err)
		}
	}

	// 2e. Associate all new tags.
	if len(tags) > 0 {
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

	// 3. Commit the transaction. If any step failed, it will be rolled back.
	if err := tx.Commit(); err != nil {
		return err
	}

	// 4. Report success for all processed files.
	for _, data := range allFileData {
		progressCb(data.path, nil)
	}

	return nil
}

// GetTagsForFile retrieves all tags for a given file.
func (s *Service) GetTagsForFile(filePath string) ([]string, error) {
	absPath, err := resolvePath(filePath)
	if err != nil {
		return nil, err
	}
	hash, err := s.Store.FindContentHashByPath(absPath)
	if err != nil {
		return nil, err
	}
	if hash == "" {
		// Return empty slice if file is not in DB, it's not an error.
		return []string{}, nil
	}

	return s.Store.GetTagsForContent(hash)
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

// GetAllTags retrieves all tags from the database.
func (s *Service) GetAllTags() ([]string, error) {
	return s.Store.GetAllTags()
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

// Relink performs a high-performance concurrent scan of the given directories,
// applies additions, and returns files that are no longer linked.
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
	knownHashes, err := s.Store.GetAllContentHashes()
	if err != nil {
		return result, fmt.Errorf("could not get known hashes: %w", err)
	}
	hashToTagsCache, err := s.Store.GetHashToTagsCacheMap()
	if err != nil {
		return result, fmt.Errorf("could not get tags cache: %w", err)
	}

	// 2. Perform the intelligent, targeted filesystem scan.
	fsLocations, filesScanned := s.scanDirsConcurrently(absDirs, sizeToHashes)
	result.Stats.FilesScanned = filesScanned

	// 3. Compute the difference ("diff") between the two states.
	toAdd := make(map[string]types.LocationInfo)
	toRemove := make([]string, 0)

	// Check for new or changed files on disk
	for path, fsInfo := range fsLocations {
		dbInfo, existsInDb := dbLocations[path]
		// Add if path is new, or if path exists but hash is different.
		if !existsInDb || dbInfo.Hash != fsInfo.Hash {
			if _, contentIsKnown := knownHashes[fsInfo.Hash]; contentIsKnown {
				fsInfo.TagsCache = hashToTagsCache[fsInfo.Hash]
				toAdd[path] = fsInfo
			}
		}
	}

	// Check for files that were removed from disk or whose content changed
	for path, dbInfo := range dbLocations {
		fsInfo, existsOnFs := fsLocations[path]
		// Remove if path no longer exists on disk, or if it exists but now has a different hash
		if !existsOnFs || fsInfo.Hash != dbInfo.Hash {
			toRemove = append(toRemove, path)
			// Build FileInfo for the CLI to display
			result.UnrelinkedFiles = append(result.UnrelinkedFiles, types.FileInfo{
				Path: path,
				Size: dbInfo.Size,
				Tags: dbInfo.TagsCache,
			})
		}
	}

	// 4. Apply only the additions.
	locationsAdded, err := s.Store.ApplyRelinkAdditions(toAdd)
	if err != nil {
		return result, err
	}
	result.Stats.LocationsAdded = locationsAdded

	return result, nil
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
