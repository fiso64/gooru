package gooru

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// buildQuery is a helper to parse an expression, gather tag statistics, and build an optimized SQL subquery.
func (c *Client) buildQuery(expression string) (string, []interface{}, error) {
	if strings.TrimSpace(expression) == "" {
		return "", nil, nil
	}
	ast, err := query.Parse(expression)
	if err != nil {
		return "", nil, fmt.Errorf("could not parse query: %w", err)
	}

	if err := query.ValidateAST(ast); err != nil {
		return "", nil, fmt.Errorf("invalid tag in query: %w", err)
	}

	// NEW: Intelligently build query using tag counts for optimization.
	// 1. Extract all user-defined tags from the query AST.
	userTags := query.ExtractTags(ast)

	// 2. Batch-fetch the usage counts for these tags from the database.
	parsedTags := query.ToParsedTags(userTags)
	tagCounts, err := c.store.BatchGetTagCounts(parsedTags)
	if err != nil {
		return "", nil, fmt.Errorf("could not fetch tag statistics for query optimization: %w", err)
	}

	// 3. Build the SQL with the counts to inform the builder's strategy.
	sqlQuery, args := query.Build(ast, tagCounts)
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

	// Path is not in DB. Check if the content is known.
	tags, err := c.store.GetTagsForContent(currentHash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to check for existing content: %w", err)
	}

	if len(tags) > 0 {
		return tags, types.StatusUntrackedContent, nil
	}

	// Path and content are both unknown.
	return []string{}, types.StatusNotInDB, nil
}

// CountFilesByQuery counts files matching a query expression.
// It uses fast, pre-calculated counts for simple single-tag queries.
func (c *Client) CountFilesByQuery(expression string, verbose bool) (int, error) {
	trimmedExpr := strings.TrimSpace(expression)
	if trimmedExpr == "" {
		return c.store.CountAllFiles()
	}

	ast, err := query.Parse(trimmedExpr)
	if err != nil {
		return 0, fmt.Errorf("could not parse query: %w", err)
	}

	if err := query.ValidateAST(ast); err != nil {
		return 0, fmt.Errorf("invalid tag in query: %w", err)
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
				if parsedTag.Key != "ext" && parsedTag.Key != "type" {
					if parsedTag.Value == "" {
						// Key-only query, e.g. `gooru count photo`
						// This must count distinct files, not sum tag counts.
						return c.store.GetCountForKey(parsedTag.Key)
					}
					// Key-value query, e.g. `gooru count "photo:album1"`
					return c.store.GetCountForTag(parsedTag.Key, parsedTag.Value)
				}
			}
		}
	}

	// Fallback to full query for complex expressions
	sqlQuery, args, err := c.buildQuery(trimmedExpr)
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

// ExistsFilesByQuery checks if any files match a query expression.
// It is optimized to be faster than counting by stopping at the first match.
func (c *Client) ExistsFilesByQuery(expression string, verbose bool) (bool, error) {
	trimmedExpr := strings.TrimSpace(expression)
	if trimmedExpr == "" {
		count, err := c.store.CountAllFiles()
		return count > 0, err
	}

	ast, err := query.Parse(trimmedExpr)
	if err != nil {
		return false, fmt.Errorf("could not parse query: %w", err)
	}

	if err := query.ValidateAST(ast); err != nil {
		return false, fmt.Errorf("invalid tag in query: %w", err)
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
				if parsedTag.Key != "ext" && parsedTag.Key != "type" {
					if parsedTag.Value == "" {
						// Key-only query, e.g., `gooru exists photo`
						return c.store.ExistsForKey(parsedTag.Key)
					}
					// Key-value query, e.g., `gooru exists "photo:album1"`
					// Using the pre-calculated count is faster than a new query.
					count, err := c.store.GetCountForTag(parsedTag.Key, parsedTag.Value)
					return count > 0, err
				}
			}
		}
	}

	// Fallback to full query for complex expressions
	sqlQuery, args, err := c.buildQuery(trimmedExpr)
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

	// Path is not in DB. Check if the content is known.
	tags, err := c.store.GetTagsForContent(currentHash)
	if err != nil {
		return types.FileInfo{Path: filePath}, 0, fmt.Errorf("failed to check for existing content: %w", err)
	}

	fileInfo := types.FileInfo{
		Path:    filePath,
		Hash:    currentHash,
		Size:    fsInfo.Size(),
		ModTime: fsInfo.ModTime().Unix(),
		Tags:    tags,
	}

	if len(tags) > 0 {
		return fileInfo, types.StatusUntrackedContent, nil
	}

	// Path and content are both unknown.
	return fileInfo, types.StatusNotInDB, nil
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

// GetAllTags retrieves all tags from the database.
func (c *Client) GetAllTags() ([]string, error) {
	return c.store.GetAllTags()
}

// GetAllTagsWithCounts retrieves all tags and their usage counts, sorted by count descending.
func (c *Client) GetAllTagsWithCounts() ([]types.TagWithCount, error) {
	return c.store.GetAllTagsWithCounts()
}
