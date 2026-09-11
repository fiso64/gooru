package gooru

// DeleteLocationsByPublicIDs removes stable public location identities in one
// transaction. Missing identities are harmless, which makes durable retries
// idempotent after the database commit boundary.
func (c *Client) DeleteLocationsByPublicIDs(publicIDs []string) (int, error) {
	if len(publicIDs) == 0 {
		return 0, nil
	}
	tx, err := c.store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	removed, err := c.store.RemoveLocationsByPublicIDTx(tx, publicIDs)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}
