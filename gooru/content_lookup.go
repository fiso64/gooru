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
	files, err := c.store.GetFilesInfoByContentQuery("SELECT ?", []interface{}{hash})
	if err != nil {
		return types.FileInfo{}, err
	}
	if len(files) == 0 {
		return types.FileInfo{}, fmt.Errorf("%w: %q", ErrContentNotTracked, hash)
	}
	return files[0], nil
}
