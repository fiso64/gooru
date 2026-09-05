package cmd

import (
	"errors"
	"fmt"
	"os"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

type registeredFileLister interface {
	GetAllFilesInfo() ([]types.FileInfo, error)
}

// ensureStorageEncryptionReady migrates registered files owned by configured
// upload targets and removes plaintext derivatives before protected mode starts
// serving requests. Arbitrary indexed media outside managed roots is
// intentionally left untouched: Gooru owns encryption-at-rest for its database,
// managed uploads, and derivative cache, not a user's external library trees.
func ensureStorageEncryptionReady(cfg serve.Config, client registeredFileLister) error {
	if !cfg.Encryption.Enabled {
		return nil
	}
	if err := serve.CleanupPlaintextMediaCache(cfg.Media); err != nil {
		return fmt.Errorf("clean plaintext media cache for protected mode: %w", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil {
		return fmt.Errorf("list registered files for encryption preflight: %w", err)
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		path := file.Path
		if !serve.IsManagedUploadPath(cfg.Uploads.Targets, path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect managed upload before encryption migration: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing protected-mode migration of managed upload symlink %q", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing protected-mode migration of non-regular managed upload %q", path)
		}
		if err := encryptedfile.EncryptFileInPlace(path, cfg.Encryption.Key); err != nil && !errors.Is(err, encryptedfile.ErrAlreadyEncrypted) {
			return fmt.Errorf("migrate managed upload to encrypted storage: %w", err)
		}
	}
	return nil
}
