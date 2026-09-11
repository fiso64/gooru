package gooru

import "gooru.local/types"

// GetFileInfosByPublicIDs resolves tracked file metadata in the exact requested
// public-ID order while batching the underlying database work.
func (c *Client) GetFileInfosByPublicIDs(publicIDs []string) ([]types.FileInfo, error) {
	return c.store.GetFileInfosByPublicIDs(publicIDs)
}
