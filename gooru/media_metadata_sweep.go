package gooru

import "gooru.local/types"

func (c *Client) ListPendingMediaMetadataContentHashes(limit int) ([]string, error) {
	return c.store.ListPendingMediaMetadataContentHashes(limit)
}

func (c *Client) GetMediaMetadataByContentHash(hash string) (types.MediaMetadata, bool, error) {
	return c.store.GetMediaMetadataByContentHash(hash)
}

func (c *Client) UpsertMediaMetadataForContentHash(hash string, meta types.MediaMetadata) error {
	return c.store.UpsertMediaMetadataForContentHash(hash, meta)
}
