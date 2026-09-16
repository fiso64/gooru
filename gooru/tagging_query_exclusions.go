package gooru

import (
	"fmt"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// TagFilesByQueryExcluding adds tags to query matches except content hashes in excludedHashes.
func (c *Client) TagFilesByQueryExcluding(expression string, tags, excludedHashes []string) (int, error) {
	return c.mutateTagsByQueryExcluding(expression, tags, excludedHashes, opTag)
}

// UntagFilesByQueryExcluding removes tags from query matches except content hashes in excludedHashes.
func (c *Client) UntagFilesByQueryExcluding(expression string, tags, excludedHashes []string) (int, error) {
	return c.mutateTagsByQueryExcluding(expression, tags, excludedHashes, opUntag)
}

// SetTagsForFilesByQueryExcluding replaces tags on query matches except content hashes in excludedHashes.
func (c *Client) SetTagsForFilesByQueryExcluding(expression string, tags, excludedHashes []string) (int, error) {
	return c.mutateTagsByQueryExcluding(expression, tags, excludedHashes, opSetTags)
}

func (c *Client) mutateTagsByQueryExcluding(expression string, tags, excludedHashes []string, operation opKind) (int, error) {
	if operation != opUntag || len(tags) > 0 {
		if err := query.ValidateTags(tags); err != nil {
			return 0, err
		}
	}
	if operation == opTag && len(tags) == 0 {
		return 0, nil
	}

	sqlQuery, args, err := c.buildQuery(expression)
	if err != nil {
		return 0, err
	}
	if sqlQuery == "" {
		return 0, nil
	}
	sqlQuery, args = excludeContentHashes(sqlQuery, args, excludedHashes)

	tx, err := c.store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if operation == opSetTags {
		tempTableQuery := fmt.Sprintf("CREATE TEMP TABLE hashes_to_update AS %s", sqlQuery)
		if _, err := tx.Exec(tempTableQuery, args...); err != nil {
			return 0, fmt.Errorf("failed to create temporary table for update: %w", err)
		}
		var affectedCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM hashes_to_update").Scan(&affectedCount); err != nil {
			return 0, fmt.Errorf("failed to count files for update: %w", err)
		}
		if affectedCount == 0 {
			return 0, tx.Commit()
		}
		if _, err := tx.Exec("DELETE FROM content_tags WHERE content_hash IN (SELECT hash FROM hashes_to_update)"); err != nil {
			return 0, fmt.Errorf("failed to clear existing tags: %w", err)
		}
		if len(tags) > 0 {
			tagIDs, err := c.tagIDsForMutation(tx, tags, true)
			if err != nil {
				return 0, err
			}
			if _, err := c.store.BatchAssociateTagsByContentQueryTx(tx, "SELECT hash FROM hashes_to_update", nil, tagIDs); err != nil {
				return 0, fmt.Errorf("failed to associate replacement tags: %w", err)
			}
		}
		return affectedCount, tx.Commit()
	}

	if operation == opUntag && len(tags) == 0 {
		countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (%s)`, sqlQuery)
		var fileCount int
		if err := tx.QueryRow(countQuery, args...).Scan(&fileCount); err != nil {
			return 0, fmt.Errorf("failed to count files for tag clearing: %w", err)
		}
		if fileCount == 0 {
			return 0, tx.Commit()
		}
		if _, err := c.store.BatchClearTagsByContentQueryTx(tx, sqlQuery, args); err != nil {
			return 0, fmt.Errorf("failed to clear tags: %w", err)
		}
		return fileCount, tx.Commit()
	}

	tagIDs, err := c.tagIDsForMutation(tx, tags, operation == opTag)
	if err != nil {
		return 0, err
	}
	if len(tagIDs) == 0 {
		return 0, tx.Commit()
	}

	var affected int64
	switch operation {
	case opTag:
		affected, err = c.store.BatchAssociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
	case opUntag:
		affected, err = c.store.BatchDisassociateTagsByContentQueryTx(tx, sqlQuery, args, tagIDs)
	default:
		return 0, fmt.Errorf("unsupported query tag operation %d", operation)
	}
	if err != nil {
		return 0, err
	}
	return int(affected), tx.Commit()
}

func (c *Client) tagIDsForMutation(tx *databaseTx, tags []string, create bool) ([]int64, error) {
	parsed := make([]types.ParsedTag, len(tags))
	for i, tag := range tags {
		parsed[i] = query.ParseTag(tag)
	}

	var (
		idsByTag map[string]int64
		err      error
	)
	if create {
		idsByTag, err = c.store.BatchGetOrCreateTags(tx, parsed)
	} else {
		idsByTag, err = c.store.BatchGetTags(tx, parsed)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tags: %w", err)
	}

	ids := make([]int64, 0, len(idsByTag))
	for _, id := range idsByTag {
		ids = append(ids, id)
	}
	return ids, nil
}

func excludeContentHashes(sqlQuery string, args []interface{}, excludedHashes []string) (string, []interface{}) {
	seen := make(map[string]struct{}, len(excludedHashes))
	filtered := make([]string, 0, len(excludedHashes))
	for _, hash := range excludedHashes {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			continue
		}
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}
		filtered = append(filtered, hash)
	}
	if len(filtered) == 0 {
		return sqlQuery, args
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(filtered)), ",")
	wrapped := fmt.Sprintf("SELECT hash FROM (%s) WHERE hash NOT IN (%s)", sqlQuery, placeholders)
	for _, hash := range filtered {
		args = append(args, hash)
	}
	return wrapped, args
}
