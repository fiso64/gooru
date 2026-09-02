package cmd

import (
	"errors"

	"gooru.local/internal/serve"
)

var errStorageEncryptionIncomplete = errors.New("encryption is enabled, but database and upload at-rest encryption are not implemented yet; refusing to start with partial protection")

// ensureStorageEncryptionReady prevents the server from advertising protected
// mode before both durable storage paths are actually encrypted. Cache and log
// hardening can land independently, but enabling encryption must never create a
// false expectation that the database or uploaded originals are protected.
func ensureStorageEncryptionReady(cfg serve.Config) error {
	if cfg.Encryption.Enabled {
		return errStorageEncryptionIncomplete
	}
	return nil
}
