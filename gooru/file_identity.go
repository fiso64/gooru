package gooru

import (
	"database/sql"
	"errors"

	"gooru.local/types"
)

// PublicFileID returns the public API identifier for a tracked file location.
func (c *Client) PublicFileID(locationID int64) string {
	id, err := c.store.GetLocationPublicID(locationID)
	if err != nil {
		return ""
	}
	return id
}

// ResolvePublicFileID resolves a public API file identifier to an internal location ID.
func (c *Client) ResolvePublicFileID(id string) (int64, error) {
	return c.store.GetLocationIDByPublicID(id)
}

// GetFileInfoByPublicID retrieves file info using the public API identifier.
func (c *Client) GetFileInfoByPublicID(id string) (types.FileInfo, error) {
	locationID, err := c.ResolvePublicFileID(id)
	if errors.Is(err, sql.ErrNoRows) {
		return types.FileInfo{}, sql.ErrNoRows
	}
	if err != nil {
		return types.FileInfo{}, err
	}
	return c.GetFileInfoByLocationID(locationID)
}
