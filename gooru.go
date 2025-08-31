package gooru

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/gooru/internal/database"
	"gooru.local/gooru/internal/hashing"
	"gooru.local/gooru/internal/query"
	"gooru.local/gooru/internal/scanning"
	"gooru.local/gooru/types"
)

type opKind int

const (
	opTag opKind = iota
	opSetTags
	opUntag
)

// Client encapsulates the core business logic.
type Client struct {
	store *database.Store
}

// New creates a new Client and initializes the database connection.
// The caller is responsible for calling Close() on the returned client.
func New(dbPath string, verbose bool) (*Client, error) {
	store, err := database.NewStore(dbPath, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	return &Client{store: store}, nil
}

// buildQuery is a helper to parse an expression and build the SQL subquery.
func (c *Client) buildQuery(expression string) (string, []interface{}, error) {
	if strings.TrimSpace(expression) == "" {
		return "", nil, nil
	}
	ast, err := query.Parse(expression)
	if err != nil {
		return "", nil, fmt.Errorf("could not parse query: %w", err)
	}

	sqlQuery, args := query.Build(ast)
	return sqlQuery, args, nil
}

// Close closes the underlying database connection.
func (c *Client) Close() error {
	if c.store != nil {
		return c.store.Close()
	}
	return nil
}

// resolvePath canonicalizes a path. If it doesn't exist, it returns the
// absolute path of the input to allow for clean "file not found" errors later.
func resolvePath(filePath string) (string, error) {
	// First, get the absolute path. This can fail if the working directory is invalid.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	// Now, check for filesystem errors other than NotExist (e.g., permission denied).
	// We ignore NotExist because the service layer is equipped to handle it.
	if _, err := os.Lstat(filePath); err != nil && !os.IsNotExist(err) {
		return "", err // Return the actual filesystem error.
	}

	return absPath, nil
}

// TagFiles adds tags to multiple files using a high-performance batching strategy.
func (c *Client) TagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	return c.performTagOperation(filePaths, tags, progressCb, opTag)
}

// UntagFiles removes tags from files. If no tags are provided, all tags are removed.
func (c *Client) UntagFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	if len(tags) == 0 {
		// Clearing all tags is equivalent to `settags` with no tags.
		return c.performTagOperation(filePaths, []string{}, progressCb, opSetTags)
	}
	return c.performTagOperation(filePaths, tags, progressCb, opUntag)
}

// TagFilesByQuery adds tags to all files matching a query expression.
// Returns the number of tags added (which may be different from files affected if tags already existed).
func (c *Client) TagFilesByQuery(expression string, tags []string) (int, error) {
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

	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
	if err != nil {
		return 0, fmt.Errorf("failed to get or create tags: %w", err)
	}

	tagIDs := make([]int64, 0, len(tags))
	for _, tagStr := range tags {
		tagIDs = append(tagIDs, tagIDMap[tagStr])
	}

	affected, err := c.store.BatchAssociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
	if err != nil {
		return 0, err
	}

	return int(affected), tx.Commit()
}

// UntagFilesByQuery removes tags from all files matching a query expression.
// If tags is empty, it removes ALL tags from matching files.
// Returns the number of tags removed.
func (c *Client) UntagFilesByQuery(expression string, tags []string) (int, error) {
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

	var affected int64
	if len(tags) == 0 {
		// Clear all tags
		affected, err = c.store.BatchClearTagsByContentQueryTx(tx, sqlQuery, args)
	} else {
		// Untag specific tags
		parsedTags := make([]types.ParsedTag, len(tags))
		for i, t := range tags {
			parsedTags[i] = query.ParseTag(t)
		}
		tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
		if err != nil {
			return 0, fmt.Errorf("failed to look up tags: %w", err)
		}

		tagIDs := make([]int64, 0, len(tags))
		for _, tagStr := range tags {
			if id, ok := tagIDMap[tagStr]; ok {
				tagIDs = append(tagIDs, id)
			}
		}

		if len(tagIDs) > 0 {
			affected, err = c.store.BatchDisassociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
		}
	}
	if err != nil {
		return 0, err
	}

	return int(affected), tx.Commit()
}

// SetTagsForFilesByQuery sets tags for all files matching a query expression, replacing existing ones.
// Returns the number of content items affected.
func (c *Client) SetTagsForFilesByQuery(expression string, tags []string) (int, error) {
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

	// 1. Clear existing tags
	if _, err := c.store.BatchClearTagsByContentQueryTx(tx, sqlQuery, args); err != nil {
		return 0, fmt.Errorf("failed to clear existing tags: %w", err)
	}

	// 2. Add new tags
	if len(tags) > 0 {
		parsedTags := make([]types.ParsedTag, len(tags))
		for i, t := range tags {
			parsedTags[i] = query.ParseTag(t)
		}
		tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
		if err != nil {
			return 0, fmt.Errorf("failed to get or create new tags: %w", err)
		}

		tagIDs := make([]int64, 0, len(tags))
		for _, tagStr := range tags {
			tagIDs = append(tagIDs, tagIDMap[tagStr])
		}

		if _, err := c.store.BatchAssociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs); err != nil {
			return 0, fmt.Errorf("failed to associate new tags: %w", err)
		}
	}

	// For Set, it's hard to get a meaningful "affected" count. The number of *files*
	// is more useful. We can get this by running a COUNT on the subquery.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", sqlQuery)
	var count int
	if err := tx.QueryRow(countQuery, args...).Scan(&count); err != nil {
		// Don't fail the whole transaction, just return 0 for the count.
		count = 0
	}

	return count, tx.Commit()
}

// SetTagsForFiles sets the tags for multiple files, replacing any existing ones, using a batching strategy.
func (c *Client) SetTagsForFiles(filePaths []string, tags []string, progressCb func(filePath string, err error)) error {
	return c.performTagOperation(filePaths, tags, progressCb, opSetTags)
}

// performTagOperation is the unified, high-performance batching logic for tag, settags, and untag.
func (c *Client) performTagOperation(filePaths []string, tags []string, progressCb func(filePath string, err error), kind opKind) error {
	// Phase 1: Collect info from FS and DB, and hash necessary files.
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
		return fmt.Errorf("could not get existing file data: %w", err)
	}

	// 1c. Determine which files actually need to be hashed.
	filesToHash := make([]string, 0)
	for _, absPath := range absPaths {
		info, err := os.Stat(absPath)
		if err != nil {
			if progressCb != nil {
				progressCb(originalPathMap[absPath], err)
			}
			continue
		}
		dbInfo, existsInDb := dbLocations[absPath]
		if existsInDb && info.Size() == dbInfo.Size && info.ModTime().Unix() == dbInfo.ModTime {
			continue // Unchanged, no hash needed.
		}
		filesToHash = append(filesToHash, absPath)
	}

	// 1d. Concurrently hash all the necessary files.
	hashResults := hashing.ConcurrentlyHashFiles(filesToHash)

	// Phase 2: Prepare data structures for transaction, identifying moves and modifications.
	type fileData struct {
		path         string // original path for callbacks
		info         types.LocationInfo
		wasModified  bool
		orphanedTags []string
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
				if progressCb != nil {
					progressCb(originalPath, err)
				}
				processedPaths[originalPath] = true
			}
			continue
		}

		var hash string
		dbInfo, existsInDb := dbLocations[absPath]
		wasHashed := false
		isModification := false

		if existsInDb && info.Size() == dbInfo.Size && info.ModTime().Unix() == dbInfo.ModTime {
			hash = dbInfo.Hash
		} else {
			result, ok := hashResults[absPath]
			if !ok || result.Err != nil {
				if !processedPaths[originalPath] {
					errMsg := "file processing failed"
					if result.Err != nil {
						errMsg = fmt.Sprintf("hashing failed: %v", result.Err)
					}
					if progressCb != nil {
						progressCb(originalPath, fmt.Errorf("%s", errMsg))
					}
					processedPaths[originalPath] = true
				}
				continue
			}
			hash = result.Hash
			wasHashed = true
			if existsInDb {
				isModification = true
			}
		}

		if wasHashed {
			potentialMoves[hash] = absPath
		}

		var orphanedTags []string
		if isModification {
			oldTags, err := c.store.GetTagsForContent(dbInfo.Hash)
			if err != nil && err != sql.ErrNoRows {
				// Log? For now, just proceed. The user operation should not fail.
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
		allFileData = append(allFileData, fileData{
			path:         originalPath,
			info:         locInfo,
			wasModified:  isModification,
			orphanedTags: orphanedTags,
		})
		allHashes = append(allHashes, hash)
		locationsToUpsert[absPath] = locInfo
	}

	if len(allFileData) == 0 {
		return nil // No files could be processed.
	}

	movesHandled := make(map[string]string) // newPath -> oldPath

	// Phase 3: The Transaction.
	tx, err := c.store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 3a: Intelligently handle file moves.
	if len(potentialMoves) > 0 {
		hashesToCheck := make([]string, 0, len(potentialMoves))
		for hash := range potentialMoves {
			hashesToCheck = append(hashesToCheck, hash)
		}
		hashToOldPaths, err := c.store.BatchGetPathsForHashes(tx, hashesToCheck)
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
					if err := c.store.UpdateLocationPath(tx, oldPath, newPath); err != nil {
						return fmt.Errorf("failed to update moved path from '%s' to '%s': %w", oldPath, newPath, err)
					}
					delete(locationsToUpsert, newPath)
					movesHandled[newPath] = oldPath // Track the handled move for notification.
					break
				}
			}
		}
	}

	// 3b: Batch upsert contents and locations.
	if err := c.store.BatchInsertContents(tx, allHashes); err != nil {
		return fmt.Errorf("failed to batch insert contents: %w", err)
	}
	if err := c.store.BatchUpsertLocations(tx, locationsToUpsert); err != nil {
		return fmt.Errorf("failed to batch upsert locations: %w", err)
	}

	// 3c: Perform the specific tagging operation.
	switch kind {
	case opTag:
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
			if err != nil {
				return fmt.Errorf("failed to get or create tags: %w", err)
			}
			pairs := make([]database.ContentTagPair, 0, len(allHashes)*len(tags))
			for _, hash := range allHashes {
				for _, tagStr := range tags {
					pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagIDMap[tagStr]})
				}
			}
			if err := c.store.BatchAssociateTags(tx, pairs); err != nil {
				return fmt.Errorf("failed to batch associate tags: %w", err)
			}
		}
	case opSetTags:
		if err := c.store.BatchClearTagsForContent(tx, allHashes); err != nil {
			return fmt.Errorf("failed to batch clear tags: %w", err)
		}
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
			if err != nil {
				return fmt.Errorf("failed to get or create tags: %w", err)
			}
			pairs := make([]database.ContentTagPair, 0, len(allHashes)*len(tags))
			for _, hash := range allHashes {
				for _, tagStr := range tags {
					pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagIDMap[tagStr]})
				}
			}
			if err := c.store.BatchAssociateTags(tx, pairs); err != nil {
				return fmt.Errorf("failed to batch associate tags: %w", err)
			}
		}
	case opUntag:
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			// For untag, we only care about tags that already exist.
			tagIDMap, err := c.store.BatchGetTags(tx, parsedTags)
			if err != nil {
				return fmt.Errorf("failed to look up tags: %w", err)
			}
			pairs := make([]database.ContentTagPair, 0, len(allHashes)*len(tags))
			for _, hash := range allHashes {
				for _, tagStr := range tags {
					if tagID, ok := tagIDMap[tagStr]; ok {
						pairs = append(pairs, database.ContentTagPair{ContentHash: hash, TagID: tagID})
					}
				}
			}
			if err := c.store.BatchDisassociateTags(tx, pairs); err != nil {
				return fmt.Errorf("failed to batch disassociate tags: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Phase 4: Notifications and Progress Callback.
	for _, data := range allFileData {
		if data.wasModified {
			fmt.Printf("Updated database for modified file: '%s'\n", data.path)
			if len(data.orphanedTags) > 0 {
				fmt.Printf("WARNING: '%s' was modified. The old version's tags [%s] are now orphaned. Run 'gooru relinkall' to find moved copies or 'gooru prune' to clean up.\n", data.path, strings.Join(data.orphanedTags, ", "))
			}
		} else if oldPath, ok := movesHandled[data.info.Path]; ok {
			// A file can't be modified and moved in the same operation, so this is an `else if`.
			// The old path is retrieved from the database, and data.path is the original user argument.
			fmt.Printf("Detected move for known content: '%s' -> '%s'\n", oldPath, data.path)
		}
	}

	if progressCb != nil {
		for _, data := range allFileData {
			progressCb(data.path, nil)
		}
	}

	return nil
}

// GetTagsForFile retrieves all tags for a given file, with a safety check and status.
func (c *Client) GetTagsForFile(filePath string) ([]string, types.FileStatus, error) {
	absPath, err := resolvePath(filePath)
	if err != nil {
		return nil, 0, err
	}

	fsInfo, err := os.Stat(absPath)
	if err != nil {
		// Propagate FS errors like permission denied or file not existing.
		return nil, 0, err
	}

	dbInfo, err := c.store.GetLocationByPath(absPath)
	if err != nil {
		if err == sql.ErrNoRows {
			// File exists on disk but not in DB.
			return []string{}, types.StatusNotInDB, nil
		}
		return nil, 0, fmt.Errorf("database lookup failed: %w", err)
	}

	// File is in DB, now check for modification.
	if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
		return []string{}, types.StatusModified, nil
	}

	// File is in DB and matches.
	tags, err := c.store.GetTagsForContent(dbInfo.Hash)
	return tags, types.StatusOK, err
}

// GetFileInfoForFile retrieves file info for a given file, with a safety check and status.
func (c *Client) GetFileInfoForFile(filePath string) (types.FileInfo, types.FileStatus, error) {
	absPath, err := resolvePath(filePath)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, err
	}

	fsInfo, err := os.Stat(absPath)
	if err != nil {
		// Propagate FS errors. Caller can handle os.IsNotExist if they want.
		return types.FileInfo{Path: filePath}, 0, err
	}

	dbInfo, err := c.store.GetLocationByPath(absPath)
	if err != nil {
		if err == sql.ErrNoRows {
			// File exists on disk but not in DB.
			return types.FileInfo{Path: filePath, Size: fsInfo.Size()}, types.StatusNotInDB, nil
		}
		return types.FileInfo{Path: filePath}, 0, fmt.Errorf("database lookup failed: %w", err)
	}

	// File is in DB, now check for modification.
	if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
		return types.FileInfo{Path: filePath, Size: fsInfo.Size()}, types.StatusModified, nil
	}

	// Matched, return full info.
	return types.FileInfo{
		Path: filePath, // use original path for display
		Size: dbInfo.Size,
		Tags: dbInfo.TagsCache,
	}, types.StatusOK, nil
}

// ListAllFiles lists all files known to the system.
func (c *Client) ListAllFiles() ([]string, error) {
	return c.store.ListAllFiles()
}

// ListFilesByTag lists all files associated with a given tag.
func (c *Client) ListFilesByTag(tag string) ([]string, error) {
	parsedTag := query.ParseTag(tag)
	return c.store.ListFilesByTag(parsedTag.Key, parsedTag.Value)
}

// ListFilesByTagsAnd lists all files associated with a given set of tags (AND query).
func (c *Client) ListFilesByTagsAnd(tags []string) ([]string, error) {
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	return c.store.ListFilesByTagsAnd(parsedTags)
}

// ListFilesByQuery parses and executes a complex query expression.
func (c *Client) ListFilesByQuery(expression string, verbose bool) ([]string, error) {
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

	return c.store.GetPathsByContentQuery(sqlQuery, args)
}

// GetAllFilesInfo gets detailed info for all files known to the system.
func (c *Client) GetAllFilesInfo() ([]types.FileInfo, error) {
	return c.store.GetAllFilesInfo()
}

// GetFilesInfoByTag gets detailed info for all files associated with a given tag.
func (c *Client) GetFilesInfoByTag(tag string) ([]types.FileInfo, error) {
	parsedTag := query.ParseTag(tag)
	return c.store.GetFilesInfoByTag(parsedTag.Key, parsedTag.Value)
}

// GetFilesInfoByTagsAnd gets detailed info for all files associated with a given set of tags (AND query).
func (c *Client) GetFilesInfoByTagsAnd(tags []string) ([]types.FileInfo, error) {
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	return c.store.GetFilesInfoByTagsAnd(parsedTags)
}

// GetFilesInfoByQuery parses and executes a complex query expression, returning full file info.
func (c *Client) GetFilesInfoByQuery(expression string, verbose bool) ([]types.FileInfo, error) {
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

	return c.store.GetFilesInfoByContentQuery(sqlQuery, args)
}

// GetAllTags retrieves all tags from the database.
func (c *Client) GetAllTags() ([]string, error) {
	return c.store.GetAllTags()
}

// GetAllTagsWithCounts retrieves all tags and their usage counts, sorted by count descending.
func (c *Client) GetAllTagsWithCounts() ([]types.TagWithCount, error) {
	return c.store.GetAllTagsWithCounts()
}

// reconcileMoves checks for and atomically updates the paths of moved files.
func (c *Client) reconcileMoves(potentialMoves map[string]string) error {
	if len(potentialMoves) == 0 {
		return nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	hashesToCheck := make([]string, 0, len(potentialMoves))
	for hash := range potentialMoves {
		hashesToCheck = append(hashesToCheck, hash)
	}

	hashToOldPaths, err := c.store.BatchGetPathsForHashes(tx, hashesToCheck)
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
				if err := c.store.UpdateLocationPath(tx, oldPath, newPath); err != nil {
					return fmt.Errorf("failed to update moved path from '%s' to '%s': %w", oldPath, newPath, err)
				}
				break // Handled move for this content hash.
			}
		}
	}

	return tx.Commit()
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

	return c.store.UpdatePath(absOldPath, absNewPath, oldPath, newPath)
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
	fsLocations, filesScanned := scanning.DirsConcurrently(absDirs, sizeToHashes)
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
		if err := c.store.UpdateLocationPath(tx, move.OldPath, move.NewPath); err != nil {
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
		newHash, err := hashing.HashFile(absPath)
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