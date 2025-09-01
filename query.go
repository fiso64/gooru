package gooru

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"gooru.local/gooru/internal/query"
	"gooru.local/gooru/types"
)

// buildQuery is a helper to parse an expression and build the SQL subquery.
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

	sqlQuery, args := query.Build(ast)
	return sqlQuery, args, nil
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
	if err := query.ValidateTag(tag); err != nil {
		return nil, err
	}
	parsedTag := query.ParseTag(tag)
	return c.store.ListFilesByTag(parsedTag.Key, parsedTag.Value)
}

// ListFilesByTagsAnd lists all files associated with a given set of tags (AND query).
func (c *Client) ListFilesByTagsAnd(tags []string) ([]string, error) {
	if err := query.ValidateTags(tags); err != nil {
		return nil, err
	}
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

	if err := query.ValidateAST(ast); err != nil {
		return nil, fmt.Errorf("invalid tag in query: %w", err)
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
	if err := query.ValidateTag(tag); err != nil {
		return nil, err
	}
	parsedTag := query.ParseTag(tag)
	return c.store.GetFilesInfoByTag(parsedTag.Key, parsedTag.Value)
}

// GetFilesInfoByTagsAnd gets detailed info for all files associated with a given set of tags (AND query).
func (c *Client) GetFilesInfoByTagsAnd(tags []string) ([]types.FileInfo, error) {
	if err := query.ValidateTags(tags); err != nil {
		return nil, err
	}
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

	if err := query.ValidateAST(ast); err != nil {
		return nil, fmt.Errorf("invalid tag in query: %w", err)
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