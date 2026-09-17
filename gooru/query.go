package gooru

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// ErrInvalidQuery marks query parse and validation errors that callers can
// safely present as client input failures.
var ErrInvalidQuery = errors.New("invalid query")

func parseAndValidateQuery(expression string) (*query.Expression, error) {
	ast, err := query.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("%w: could not parse query: %v", ErrInvalidQuery, err)
	}
	if err := query.ValidateAST(ast); err != nil {
		return nil, fmt.Errorf("%w: invalid tag in query: %v", ErrInvalidQuery, err)
	}
	return ast, nil
}

func (c *Client) buildQueryAST(ast *query.Expression) (string, []interface{}, error) {
	// NEW: Intelligently build query using tag counts for optimization.
	// 1. Extract all user-defined tags from the query AST.
	userTags := query.ExtractTags(ast)

	// 2. Batch-fetch the usage counts for these tags from the database.
	tagCounts, err := c.store.BatchGetQueryTagCounts(userTags)
	if err != nil {
		return "", nil, fmt.Errorf("could not fetch tag statistics for query optimization: %w", err)
	}

	// 3. Build the SQL with the counts to inform the builder's strategy.
	sqlQuery, args := query.Build(ast, tagCounts)
	return sqlQuery, args, nil
}

// buildQuery is a helper to parse an expression, gather tag statistics, and build an optimized SQL subquery.
func (c *Client) buildQuery(expression string) (string, []interface{}, error) {
	if strings.TrimSpace(expression) == "" {
		return "", nil, nil
	}
	ast, err := parseAndValidateQuery(expression)
	if err != nil {
		return "", nil, err
	}
	return c.buildQueryAST(ast)
}

// buildLocationQuery parses an expression into a SQL subquery that returns
// matching location IDs for browse/search endpoints.
func (c *Client) buildLocationQuery(expression string) (string, []interface{}, error) {
	if strings.TrimSpace(expression) == "" {
		return "", nil, nil
	}
	ast, err := parseAndValidateQuery(expression)
	if err != nil {
		return "", nil, err
	}
	userTags := query.ExtractTags(ast)
	tagCounts, err := c.store.BatchGetQueryTagCounts(userTags)
	if err != nil {
		return "", nil, fmt.Errorf("could not fetch tag statistics for query optimization: %w", err)
	}
	sqlQuery, args := query.BuildLocations(ast, tagCounts)
	return sqlQuery, args, nil
}

// GetTagsForFile retrieves all tags for a given file, with a safety check and status.
func (c *Client) GetTagsForFile(filePath string, useMetadataHeuristic bool) ([]string, types.FileStatus, error) {
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
	if err != nil && err != sql.ErrNoRows {
		return nil, 0, fmt.Errorf("database lookup failed: %w", err)
	}
	pathInDb := err != sql.ErrNoRows

	// --- Heuristic Path (Path-Centric) ---
	if useMetadataHeuristic {
		if !pathInDb {
			return []string{}, types.StatusNotInDB, nil
		}
		if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
			return []string{}, types.StatusModified, nil
		}
		tags, err := c.store.GetTagsForContent(dbInfo.Hash)
		return tags, types.StatusOK, err
	}

	// --- Default Path (Content-Centric) ---
	currentHash, err := c.hasher.HashFile(absPath)
	if err != nil {
		return nil, 0, fmt.Errorf("could not hash file for verification: %w", err)
	}

	if pathInDb {
		if currentHash == dbInfo.Hash {
			tags, err := c.store.GetTagsForContent(dbInfo.Hash)
			return tags, types.StatusOK, err
		}
		// Path is known, but content has changed.
		return []string{}, types.StatusModified, nil
	}

	// Path is not in DB. Classify the content independently of whether it has tags.
	return c.getUntrackedContentStatus(currentHash)
}

// CountFilesByQuery counts files matching a query expression.
// It uses fast, pre-calculated counts for simple single-tag queries.
func (c *Client) CountFilesByQuery(expression string, verbose bool) (int, error) {
	trimmedExpr := strings.TrimSpace(expression)
	if trimmedExpr == "" {
		return c.store.CountAllFiles()
	}

	ast, err := parseAndValidateQuery(trimmedExpr)
	if err != nil {
		return 0, err
	}

	// Optimization for simple, single-tag queries
	if len(ast.Or) == 1 && len(ast.Or[0].And) == 1 {
		term := ast.Or[0].And[0]
		if !term.Not && term.Factor.SubExpr == nil && term.Factor.Tag != nil {
			tagStr := *term.Factor.Tag
			// This optimization does not apply to meta-tags like @tagged
			if !strings.HasPrefix(tagStr, "@") {
				parsedTag := query.ParseTag(tagStr)
				// This optimization only applies to user tags, not virtual tags like ext:
				if parsedTag.Key != "ext" && parsedTag.Key != "type" && (parsedTag.Value != "" || strings.HasSuffix(tagStr, ":")) {
					return c.store.GetCountForTag(parsedTag.Key, parsedTag.Value)
				}
			}
		}
	}

	// Fallback to full query for complex expressions using the already parsed AST.
	sqlQuery, args, err := c.buildQueryAST(ast)
	if err != nil {
		return 0, err
	}
	if sqlQuery == "" {
		return 0, nil
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\n")
		fmt.Fprintf(os.Stderr, "Expression: %s\n", expression)
		fmt.Fprintf(os.Stderr, "Built SQL : %s\n", sqlQuery)
		fmt.Fprintf(os.Stderr, "SQL Args  : %v\n", args)
		fmt.Fprintf(os.Stderr, "-------------\n")
	}

	return c.store.GetCountByContentQuery(sqlQuery, args)
}

// CountFileLocationsByQuery counts tracked file locations matching a query expression.
func (c *Client) CountFileLocationsByQuery(expression string, verbose bool) (int, error) {
	trimmedExpr := strings.TrimSpace(expression)
	if trimmedExpr == "" {
		return c.store.CountAllFiles()
	}
	sqlQuery, args, err := c.buildLocationQuery(trimmedExpr)
	if err != nil {
		return 0, err
	}
	if sqlQuery == "" {
		return 0, nil
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\nExpression: %s\nBuilt SQL : %s\nSQL Args  : %v\n-------------\n", expression, sqlQuery, args)
	}
	return c.store.GetCountByLocationQuery(sqlQuery, args)
}

// ExistsFilesByQuery checks if any files match a query expression.
// It is optimized to be faster than counting by stopping at the first match.
func (c *Client) ExistsFilesByQuery(expression string, verbose bool) (bool, error) {
	trimmedExpr := strings.TrimSpace(expression)
	if trimmedExpr == "" {
		count, err := c.store.CountAllFiles()
		return count > 0, err
	}

	ast, err := parseAndValidateQuery(trimmedExpr)
	if err != nil {
		return false, err
	}

	// Optimization for simple, single-tag queries
	if len(ast.Or) == 1 && len(ast.Or[0].And) == 1 {
		term := ast.Or[0].And[0]
		if !term.Not && term.Factor.SubExpr == nil && term.Factor.Tag != nil {
			tagStr := *term.Factor.Tag
			// This optimization does not apply to meta-tags like @tagged
			if !strings.HasPrefix(tagStr, "@") {
				parsedTag := query.ParseTag(tagStr)
				// This optimization only applies to user tags, not virtual tags like ext:
				if parsedTag.Key != "ext" && parsedTag.Key != "type" && (parsedTag.Value != "" || strings.HasSuffix(tagStr, ":")) {
					// Using the pre-calculated count is faster than a new query.
					count, err := c.store.GetCountForTag(parsedTag.Key, parsedTag.Value)
					return count > 0, err
				}
			}
		}
	}

	// Fallback to full query for complex expressions using the already parsed AST.
	sqlQuery, args, err := c.buildQueryAST(ast)
	if err != nil {
		return false, err
	}
	if sqlQuery == "" {
		return false, nil
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\n")
		fmt.Fprintf(os.Stderr, "Expression: %s\n", expression)
		fmt.Fprintf(os.Stderr, "Built SQL : %s\n", sqlQuery)
		fmt.Fprintf(os.Stderr, "SQL Args  : %v\n", args)
		fmt.Fprintf(os.Stderr, "-------------\n")
	}

	return c.store.ExistsByContentQuery(sqlQuery, args)
}

// GetFileInfoForFile retrieves file info for a given file, with a safety check and status.
func (c *Client) GetFileInfoForFile(filePath string, useMetadataHeuristic bool) (types.FileInfo, types.FileStatus, error) {
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
	if err != nil && err != sql.ErrNoRows {
		return types.FileInfo{Path: filePath}, 0, fmt.Errorf("database lookup failed: %w", err)
	}
	pathInDb := err != sql.ErrNoRows

	// --- Heuristic Path (Path-Centric) ---
	if useMetadataHeuristic {
		if !pathInDb {
			return types.FileInfo{Path: filePath, Size: fsInfo.Size()}, types.StatusNotInDB, nil
		}
		if fsInfo.Size() != dbInfo.Size || fsInfo.ModTime().Unix() != dbInfo.ModTime {
			return types.FileInfo{Path: filePath, Size: fsInfo.Size()}, types.StatusModified, nil
		}
		tags, err := c.store.GetTagsForContent(dbInfo.Hash)
		if err != nil {
			return types.FileInfo{Path: filePath}, 0, err
		}
		return types.FileInfo{Path: filePath, Hash: dbInfo.Hash, Size: dbInfo.Size, ModTime: dbInfo.ModTime, Tags: tags}, types.StatusOK, nil
	}

	// --- Default Path (Content-Centric) ---
	currentHash, err := c.hasher.HashFile(absPath)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, fmt.Errorf("could not hash file for verification: %w", err)
	}

	if pathInDb {
		if currentHash == dbInfo.Hash {
			tags, err := c.store.GetTagsForContent(dbInfo.Hash)
			if err != nil {
				return types.FileInfo{Path: filePath}, 0, err
			}
			return types.FileInfo{Path: filePath, Hash: dbInfo.Hash, Size: dbInfo.Size, ModTime: dbInfo.ModTime, Tags: tags}, types.StatusOK, nil
		}
		// Path is known, but content has changed.
		return types.FileInfo{Path: filePath, Size: fsInfo.Size()}, types.StatusModified, nil
	}

	tags, status, err := c.getUntrackedContentStatus(currentHash)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, err
	}
	fileInfo := types.FileInfo{
		Path:    filePath,
		Hash:    currentHash,
		Size:    fsInfo.Size(),
		ModTime: fsInfo.ModTime().Unix(),
		Tags:    tags,
	}
	return fileInfo, status, nil
}

// GetFileInfoForSource computes content identity and status from a supplied
// plaintext random-access source while treating filePath as the logical storage
// location. It mirrors the content-centric branch of GetFileInfoForFile without
// requiring the logical path itself to contain plaintext bytes.
func (c *Client) GetFileInfoForSource(filePath string, source io.ReaderAt, size int64, modTime int64) (types.FileInfo, types.FileStatus, error) {
	absPath, err := resolvePath(filePath)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, err
	}
	currentHash, err := c.hasher.HashSource(source, size)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, fmt.Errorf("could not hash file source for verification: %w", err)
	}
	dbInfo, err := c.store.GetLocationByPath(absPath)
	if err != nil && err != sql.ErrNoRows {
		return types.FileInfo{Path: filePath}, 0, fmt.Errorf("database lookup failed: %w", err)
	}
	pathInDB := err != sql.ErrNoRows
	if pathInDB {
		if currentHash != dbInfo.Hash {
			return types.FileInfo{Path: filePath, Hash: currentHash, Size: size, ModTime: modTime}, types.StatusModified, nil
		}
		tags, err := c.store.GetTagsForContent(dbInfo.Hash)
		if err != nil {
			return types.FileInfo{Path: filePath}, 0, err
		}
		return types.FileInfo{Path: filePath, Hash: dbInfo.Hash, Size: size, ModTime: modTime, Tags: tags}, types.StatusOK, nil
	}
	tags, status, err := c.getUntrackedContentStatus(currentHash)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, err
	}
	info := types.FileInfo{Path: filePath, Hash: currentHash, Size: size, ModTime: modTime, Tags: tags}
	return info, status, nil
}

// ContentExists reports whether a content hash is already tracked.
func (c *Client) ContentExists(hash string) (bool, error) {
	return c.store.ContentExists(hash)
}

// ListAllFiles lists all files known to the system.
func (c *Client) ListAllFiles() ([]string, error) {
	return c.store.ListAllFiles()
}

// ListFilesByTag lists all files associated with a given tag.
func (c *Client) ListFilesByTag(tag string) ([]string, error) {
	if err := query.ValidateTag(tag); err != nil {
		return nil, err
	}
	parsedTag := query.ParseTag(tag)
	return c.store.ListFilesByTag(parsedTag.Key, parsedTag.Value)
}

// ListFilesByTagsAnd lists all files associated with a given set of tags (AND query),
// while excluding any files that have any of the specified notTags.
func (c *Client) ListFilesByTagsAnd(tags []string, notTags []string) ([]string, error) {
	if err := query.ValidateTags(tags); err != nil {
		return nil, err
	}
	// Note: We use ValidateTags here, which disallows query-specific characters like '*'.
	// This is correct as this helper method is for simple, direct tag matching.
	if err := query.ValidateTags(notTags); err != nil {
		return nil, err
	}
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	parsedNotTags := make([]types.ParsedTag, len(notTags))
	for i, t := range notTags {
		parsedNotTags[i] = query.ParseTag(t)
	}
	return c.store.ListFilesByTagsAnd(parsedTags, parsedNotTags)
}

// ListFilesByQuery parses and executes a complex query expression.
func (c *Client) ListFilesByQuery(expression string, verbose bool) ([]string, error) {
	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return nil, err
	}
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

// GetAllFilesInfoPage gets one bounded page of files known to the system.
func (c *Client) GetAllFilesInfoPage(limit int, offset int) ([]types.FileInfo, error) {
	return c.store.GetAllFilesInfoPage(limit, offset)
}

// GetFileInfoByLocationID gets detailed info for a tracked file by its stable location ID.
func (c *Client) GetFileInfoByLocationID(id int64) (types.FileInfo, error) {
	return c.store.GetFileInfoByLocationID(id)
}

// GetFilesInfoByTag gets detailed info for all files associated with a given tag.
func (c *Client) GetFilesInfoByTag(tag string) ([]types.FileInfo, error) {
	if err := query.ValidateTag(tag); err != nil {
		return nil, err
	}
	parsedTag := query.ParseTag(tag)
	return c.store.GetFilesInfoByTag(parsedTag.Key, parsedTag.Value)
}

// GetFilesInfoByTagsAnd gets detailed info for all files associated with a given set of tags (AND query),
// while excluding any files that have any of the specified notTags.
func (c *Client) GetFilesInfoByTagsAnd(tags []string, notTags []string) ([]types.FileInfo, error) {
	if err := query.ValidateTags(tags); err != nil {
		return nil, err
	}
	if err := query.ValidateTags(notTags); err != nil {
		return nil, err
	}
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, t := range tags {
		parsedTags[i] = query.ParseTag(t)
	}
	parsedNotTags := make([]types.ParsedTag, len(notTags))
	for i, t := range notTags {
		parsedNotTags[i] = query.ParseTag(t)
	}
	return c.store.GetFilesInfoByTagsAnd(parsedTags, parsedNotTags)
}

// GetFilesInfoByQuery parses and executes a complex query expression, returning full file info.
func (c *Client) GetFilesInfoByQuery(expression string, verbose bool) ([]types.FileInfo, error) {
	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return nil, err
	}
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

// ListPublicFileIDsByQuery returns only stable public IDs for matching tracked locations.
// It deliberately avoids materializing FileInfo rows for large immutable selections.
func (c *Client) ListPublicFileIDsByQuery(expression string, verbose bool) ([]string, error) {
	sqlQuery, args, err := c.buildLocationQuery(expression)
	if err != nil {
		return nil, err
	}
	if sqlQuery == "" {
		return c.store.ListAllPublicFileIDs()
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\nExpression: %s\nBuilt SQL : %s\nSQL Args  : %v\n-------------\n", expression, sqlQuery, args)
	}
	return c.store.GetPublicFileIDsByLocationQuery(sqlQuery, args)
}

// GetFilesInfoByQueryPage parses and executes a query expression with bounded pagination.
func (c *Client) GetFilesInfoByQueryPage(expression string, limit int, offset int, verbose bool) ([]types.FileInfo, error) {
	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return nil, err
	}
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

	return c.store.GetFilesInfoByContentQueryPage(sqlQuery, args, limit, offset)
}

func (c *Client) GetFilesInfoByQueryPageSorted(expression string, limit int, cursor *types.PageCursor, sort string, order string, verbose bool) ([]types.FileInfo, error) {
	sqlQuery, args, err := c.buildLocationQuery(expression)
	if err != nil {
		return nil, err
	}
	if sqlQuery == "" {
		return c.store.GetAllFilesInfoPageSortedAddedOrder(limit, cursor, sort, order)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\nExpression: %s\nBuilt SQL : %s\nSQL Args  : %v\n-------------\n", expression, sqlQuery, args)
	}
	return c.store.GetFilesInfoByLocationQueryPageSortedAddedOrder(sqlQuery, args, limit, cursor, sort, order)
}

func (c *Client) GetFilesInfoByQueryPageSortedOffset(expression string, limit int, offset int, sort string, order string, verbose bool) ([]types.FileInfo, error) {
	sqlQuery, args, err := c.buildLocationQuery(expression)
	if err != nil {
		return nil, err
	}
	if sqlQuery == "" {
		return c.store.GetAllFilesInfoPageSortedOffsetAddedOrder(limit, offset, sort, order)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\nExpression: %s\nBuilt SQL : %s\nSQL Args  : %v\n-------------\n", expression, sqlQuery, args)
	}
	return c.store.GetFilesInfoByLocationQueryPageSortedOffsetAddedOrder(sqlQuery, args, limit, offset, sort, order)
}

func (c *Client) KindFacets() ([]types.TagWithCount, error) {
	return c.store.KindFacets()
}

func (c *Client) KindFacetsByQuery(expression string, verbose bool) ([]types.TagWithCount, error) {
	if kind, ok := simpleKindFacetFilter(expression); ok {
		facets, err := c.store.KindFacets()
		if err != nil {
			return nil, err
		}
		return filterKindFacet(facets, kind), nil
	}
	if filter, ok := parseSimpleUserTagFacetFilter(expression); ok {
		if filter.KeyOnly {
			return c.store.KindFacetsForTagKey(filter.Tag.Key)
		}
		return c.store.KindFacetsForTag(filter.Tag.Key, filter.Tag.Value)
	}
	if exclusions, ok := parseSimpleUserTagFacetExclusions(expression); ok {
		return c.kindFacetsForUserTagExclusions(exclusions)
	}
	sqlQuery, args, err := c.buildLocationQuery(expression)
	if err != nil {
		return nil, err
	}
	if sqlQuery == "" {
		return c.store.KindFacets()
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "--- DEBUG ---\nExpression: %s\nBuilt SQL : %s\nSQL Args  : %v\n-------------\n", expression, sqlQuery, args)
	}
	return c.store.KindFacetsByLocationQuery(sqlQuery, args)
}

func (c *Client) TagSuggestions(prefix string, limit int) ([]types.TagWithCount, error) {
	return c.store.ListTagSuggestions(prefix, limit)
}

func (c *Client) NamespaceSuggestions(prefix string, limit int) ([]types.TagWithCount, error) {
	return c.store.ListNamespaceSuggestions(prefix, limit)
}

func (c *Client) TagValueSuggestions(namespace string, valuePrefix string, limit int) ([]types.TagWithCount, error) {
	return c.store.ListTagValueSuggestions(namespace, valuePrefix, limit)
}

func (c *Client) TagNamespaces() ([]string, error) {
	return c.store.ListTagNamespaces()
}

func (c *Client) DeleteLocationByID(id int64) (bool, error) {
	return c.store.DeleteLocationByID(id)
}

func (c *Client) GetFileInfoByPath(path string) (types.FileInfo, error) {
	return c.store.GetFileInfoByPath(path)
}

func (c *Client) GetMediaMetadata(locationID int64) (types.MediaMetadata, error) {
	return c.store.GetMediaMetadata(locationID)
}

func (c *Client) UpsertMediaMetadata(meta types.MediaMetadata) error {
	return c.store.UpsertMediaMetadata(meta)
}

func (c *Client) ListSavedSearches(userID string) ([]types.SavedSearch, error) {
	return c.store.ListSavedSearches(userID)
}

func (c *Client) CreateSavedSearch(item types.SavedSearch) (types.SavedSearch, error) {
	return c.store.CreateSavedSearch(item)
}

func (c *Client) UpdateSavedSearch(item types.SavedSearch) (types.SavedSearch, error) {
	return c.store.UpdateSavedSearch(item)
}

func (c *Client) DeleteSavedSearch(userID string, id string) (bool, error) {
	return c.store.DeleteSavedSearch(userID, id)
}

// GetAllTags retrieves all tags from the database.
func (c *Client) GetAllTags() ([]string, error) {
	return c.GetTags(0)
}

// GetTags retrieves tags from the database, optionally bounded by limit.
func (c *Client) GetTags(limit int) ([]string, error) {
	return c.store.GetTags(limit)
}

// GetAllTagsWithCounts retrieves all tags and their usage counts, sorted by count descending.
func (c *Client) GetAllTagsWithCounts() ([]types.TagWithCount, error) {
	return c.GetTagsWithCounts(0)
}

// GetTagsWithCounts retrieves tags and counts, optionally bounded by limit.
func (c *Client) GetTagsWithCounts(limit int) ([]types.TagWithCount, error) {
	return c.store.GetTagsWithCounts(limit)
}
