package cmd

import (
	"errors"
	"fmt"
	"os"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

type registeredFileLister interface {
	GetAllFilesInfo() ([]types.FileInfo, error)
}

type managedStorageRegistry interface {
	registeredFileLister
	ManagedStoragePath(locationID int64) (string, bool, error)
	SetManagedStoragePath(locationID int64, physicalPath string) error
	ClearManagedStoragePath(locationID int64) error
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
	registry, ok := client.(managedStorageRegistry)
	if !ok {
		return fmt.Errorf("protected managed storage registry is unavailable")
	}
	keys, err := encryptionkeys.Derive(cfg.Encryption.Key)
	if err != nil {
		return fmt.Errorf("derive protected-storage subkeys: %w", err)
	}
	if err := serve.CleanupPlaintextMediaCache(cfg.Media); err != nil {
		return fmt.Errorf("clean plaintext media cache for protected mode: %w", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil {
		return fmt.Errorf("list registered files for encryption preflight: %w", err)
	}
	seen := make(map[int64]struct{}, len(files))
	for _, file := range files {
		logicalPath := file.Path
		if !serve.IsManagedUploadPath(cfg.Uploads.Targets, logicalPath) {
			continue
		}
		if _, ok := seen[file.ID]; ok {
			continue
		}
		seen[file.ID] = struct{}{}

		physicalPath, mapped, err := registry.ManagedStoragePath(file.ID)
		if err != nil {
			return fmt.Errorf("read managed storage mapping for %q: %w", logicalPath, err)
		}
		if !mapped {
			physicalPath, err = serve.OpaqueManagedStoragePath(logicalPath, file.Hash, keys.Media)
			if err != nil {
				return fmt.Errorf("derive opaque managed storage path for %q: %w", logicalPath, err)
			}
		}

		logicalExists, err := regularManagedFileExists(logicalPath)
		if err != nil {
			return err
		}
		physicalExists, err := regularManagedFileExists(physicalPath)
		if err != nil {
			return err
		}
		if logicalExists && physicalExists && logicalPath != physicalPath {
			return fmt.Errorf("both logical and opaque managed upload paths exist for %q", logicalPath)
		}

		switch {
		case physicalExists:
			if err := migrateManagedFileEncryption(physicalPath, cfg.Encryption.Key, keys.Media); err != nil {
				return err
			}
			if !mapped {
				if err := registry.SetManagedStoragePath(file.ID, physicalPath); err != nil {
					return fmt.Errorf("recover opaque managed storage mapping for %q: %w", logicalPath, err)
				}
			}
		case logicalExists:
			if err := migrateManagedFileEncryption(logicalPath, cfg.Encryption.Key, keys.Media); err != nil {
				return err
			}
			if logicalPath != physicalPath {
				if err := os.Rename(logicalPath, physicalPath); err != nil {
					return fmt.Errorf("rename managed upload to opaque storage: %w", err)
				}
			}
			if err := registry.SetManagedStoragePath(file.ID, physicalPath); err != nil {
				return fmt.Errorf("record opaque managed storage mapping for %q: %w", logicalPath, err)
			}
		default:
			// Preserve the previous preflight behavior for genuinely missing files;
			// relink/recovery can disposition them without startup inventing data.
			continue
		}
	}
	return nil
}

func regularManagedFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect managed upload %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("refusing protected-mode migration of managed upload symlink %q", path)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("refusing protected-mode migration of non-regular managed upload %q", path)
	}
	return true, nil
}

func migrateManagedFileEncryption(path string, masterKey, mediaKey []byte) error {
	err := encryptedfile.EncryptFileInPlace(path, mediaKey)
	if err == nil || errors.Is(err, encryptedfile.ErrAlreadyEncrypted) {
		return nil
	}
	if errors.Is(err, encryptedfile.ErrAuthentication) {
		if migrateErr := encryptedfile.ReencryptFileInPlace(path, masterKey, mediaKey); migrateErr == nil {
			return nil
		} else {
			return fmt.Errorf("migrate managed upload to media-domain key: %w", migrateErr)
		}
	}
	return fmt.Errorf("migrate managed upload to encrypted storage: %w", err)
}
