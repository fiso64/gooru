package gooru

import (
	"errors"
	"fmt"

	"gooru.local/types"
)

// ErrContentNotTracked reports that immutable content identity no longer has a
// tracked location. Background work can use this to distinguish obsolete work
// from a real lookup/database failure without matching error strings.
var ErrContentNotTracked = errors.New("content is no longer tracked")

// GetFileInfoByContentHash resolves a currently tracked location for immutable
// content identity. The store's content query orders paths deterministically, so
// background work survives moves without persisting a stale filesystem path.
func (c *Client) GetFileInfoByContentHash(hash string) (types.FileInfo, error) {
	files, err := c.GetFileInfosByContentHash(hash)
	if err != nil {
		return types.FileInfo{}, err
	}
	if len(files) == 0 {
		return types.FileInfo{}, fmt.Errorf("%w: %q", ErrContentNotTracked, hash)
	}
	return files[0], nil
}

// GetFileInfosByContentHash resolves every currently tracked location for one
// immutable content identity. This is useful for background work that can fall
// back to another path when one duplicate location is missing or unreadable.
func (c *Client) GetFileInfosByContentHash(hash string) ([]types.FileInfo, error) {
	return c.store.GetFilesInfoByContentQuery("SELECT ?", []interface{}{hash})
}

// GetFileInfosByContentHashes resolves one deterministic tracked location for
// each requested content hash. Missing hashes are omitted from the returned map.
func (c *Client) GetFileInfosByContentHashes(hashes []string) (map[string]types.FileInfo, error) {
	return c.store.GetFileInfosByContentHashes(hashes)
}

// GetFileInfosByPaths resolves currently tracked locations by exact path.
// Missing paths are omitted from the returned map.
func (c *Client) GetFileInfosByPaths(paths []string) (map[string]types.FileInfo, error) {
	return c.store.GetFileInfosByPaths(paths)
}
