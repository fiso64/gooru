package gooru

import (
	"fmt"

	"gooru.local/types"
)

// GetFileInfoByContentHash resolves a currently tracked location for immutable
// content identity. The store's content query orders paths deterministically, so
// background work survives moves without persisting a stale filesystem path.
func (c *Client) GetFileInfoByContentHash(hash string) (types.FileInfo, error) {
	files, err := c.store.GetFilesInfoByContentQuery("SELECT ?", []interface{}{hash})
	if err != nil {
		return types.FileInfo{}, err
	}
	if len(files) == 0 {
		return types.FileInfo{}, fmt.Errorf("content %q has no tracked location", hash)
	}
	return files[0], nil
}
