package gooru

import (
	"fmt"

	"gooru.local/types"
)

// getUntrackedContentStatus distinguishes unknown content from known content
// that simply has no tags. GetTagsForContent alone cannot make that distinction
// because both cases legitimately return an empty tag slice.
func (c *Client) getUntrackedContentStatus(hash string) ([]string, types.FileStatus, error) {
	tags, err := c.store.GetTagsForContent(hash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to check for existing content: %w", err)
	}
	if len(tags) > 0 {
		return tags, types.StatusUntrackedContent, nil
	}

	exists, err := c.store.ContentExists(hash)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to check for existing content: %w", err)
	}
	if exists {
		return tags, types.StatusUntrackedContent, nil
	}
	return tags, types.StatusNotInDB, nil
}
