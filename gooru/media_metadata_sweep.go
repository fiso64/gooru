package gooru

import "gooru.local/types"

func (c *Client) ListPendingMediaMetadataFiles(afterLocationID int64, limit int) ([]types.FileInfo, error) {
	return c.store.ListPendingMediaMetadataFiles(afterLocationID, limit)
}

func (c *Client) UpsertMediaMetadataForLocation(locationID int64, expectedHash, expectedPath string, meta types.MediaMetadata) (bool, error) {
	return c.store.UpsertMediaMetadataForLocation(locationID, expectedHash, expectedPath, meta)
}
