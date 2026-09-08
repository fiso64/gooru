package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	referencedProtectedStorage := make([]string, 0, len(files))
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
			return fmt.Errorf("read managed storage mapping: %w", err)
		}
		logicalExists, err := regularManagedFileExists(logicalPath)
		if err != nil {
			return err
		}
		if !mapped {
			// Do not invent a mapping for a genuinely missing managed file. That
			// would hide the logical path from relink/recovery without protecting
			// any bytes. Once a file exists, checkpoint its random destination
			// before mutating the filesystem so a restart resumes the same rename.
			if !logicalExists {
				continue
			}
			physicalPath, err = serve.OpaqueManagedStoragePathForTargets(cfg.Uploads.Targets, logicalPath, file.Hash, keys.Media)
			if err != nil {
				return fmt.Errorf("choose opaque managed storage path: %w", err)
			}
			if err := registry.SetManagedStoragePath(file.ID, physicalPath); err != nil {
				return fmt.Errorf("checkpoint opaque managed storage path: %w", err)
			}
			mapped = true
		}

		// #440 stored opaque files beside their logical paths. Move those
		// existing mappings into the reserved versioned namespace while keeping
		// the same random opaque basename. The destination mapping is checkpointed
		// first; if the process dies before rename, the next startup reconstructs
		// the old sibling from that basename and resumes safely.
		if mapped && physicalPath != logicalPath && !serve.IsProtectedManagedStoragePath(cfg.Uploads.Targets, physicalPath) {
			legacyPath := physicalPath
			namespacedPath, err := serve.ProtectedManagedStoragePathForName(cfg.Uploads.Targets, logicalPath, filepath.Base(legacyPath))
			if err != nil {
				return fmt.Errorf("migrate legacy opaque managed storage mapping: %w", err)
			}
			legacyExists, err := regularManagedFileExists(legacyPath)
			if err != nil {
				return err
			}
			namespacedExists, err := regularManagedFileExists(namespacedPath)
			if err != nil {
				return err
			}
			if legacyExists && namespacedExists {
				return fmt.Errorf("both legacy and namespaced protected managed storage entries exist")
			}
			if err := registry.SetManagedStoragePath(file.ID, namespacedPath); err != nil {
				return fmt.Errorf("checkpoint namespaced managed storage path: %w", err)
			}
			if legacyExists && !namespacedExists {
				if err := os.Rename(legacyPath, namespacedPath); err != nil {
					return fmt.Errorf("move legacy opaque managed upload into protected namespace: %w", err)
				}
			}
			physicalPath = namespacedPath
		}

		physicalExists, err := regularManagedFileExists(physicalPath)
		if err != nil {
			return err
		}
		if mapped && serve.IsProtectedManagedStoragePath(cfg.Uploads.Targets, physicalPath) && !physicalExists && !logicalExists {
			// Recovery for a crash after checkpointing a #440 sibling's namespaced
			// destination but before moving the file itself.
			legacySibling := filepath.Join(filepath.Dir(logicalPath), filepath.Base(physicalPath))
			legacyExists, err := regularManagedFileExists(legacySibling)
			if err != nil {
				return err
			}
			if legacyExists {
				if err := os.Rename(legacySibling, physicalPath); err != nil {
					return fmt.Errorf("resume protected namespace migration: %w", err)
				}
				physicalExists = true
			}
		}
		if logicalExists && physicalExists && logicalPath != physicalPath {
			return fmt.Errorf("both logical and opaque managed upload paths exist")
		}

		switch {
		case physicalExists:
			if err := migrateManagedFileEncryption(physicalPath, cfg.Encryption.Key, keys.Media); err != nil {
				return err
			}
		case logicalExists:
			if err := migrateManagedFileEncryption(logicalPath, cfg.Encryption.Key, keys.Media); err != nil {
				return err
			}
			if logicalPath != physicalPath {
				if err := os.Rename(logicalPath, physicalPath); err != nil {
					return fmt.Errorf("rename managed upload to opaque storage: %w", err)
				}
				physicalExists = true
			}
		default:
			// A pre-existing mapping with neither path present is left intact for
			// explicit missing-file disposition; no filename is synthesized here.
			continue
		}
		if physicalExists && serve.IsProtectedManagedStoragePath(cfg.Uploads.Targets, physicalPath) {
			referencedProtectedStorage = append(referencedProtectedStorage, physicalPath)
		}
	}
	if err := serve.CleanupProtectedManagedStorageOrphans(cfg.Uploads.Targets, referencedProtectedStorage); err != nil {
		return fmt.Errorf("reconcile protected managed storage orphans: %w", err)
	}
	return nil
}

func regularManagedFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect managed upload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("refusing protected-mode migration of managed upload symlink")
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("refusing protected-mode migration of non-regular managed upload")
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
