package serve

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	opaqueManagedNameBytes    = 24
	protectedManagedNamespace = ".gooru-protected-v1"
)

// ProtectedManagedStoragePathForName returns the reserved physical path for one
// opaque managed-storage name. Protected files live beneath a versioned hidden
// namespace at the upload-target root and are sharded by the first two hex
// characters so the physical tree never mirrors logical library directories.
func ProtectedManagedStoragePathForName(targets []UploadTarget, logicalPath, opaqueName string) (string, error) {
	root, ok, err := managedUploadTargetRoot(targets, logicalPath)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("logical managed upload is outside configured targets")
	}
	return protectedManagedStoragePathForRoot(root, opaqueName)
}

// OpaqueManagedStoragePath chooses a fresh extensionless physical name for a
// newly uploaded managed file. Upload staging stores logical files directly in
// the configured target root, so the logical parent is the namespace root. The
// target-aware variant is used by startup migration for nested legacy paths.
func OpaqueManagedStoragePath(logicalPath string, _ string, _ []byte) (string, error) {
	return opaqueManagedStoragePathForRoot(filepath.Dir(logicalPath))
}

// OpaqueManagedStoragePathForTargets chooses a fresh namespace path rooted at
// the configured managed target that owns logicalPath.
func OpaqueManagedStoragePathForTargets(targets []UploadTarget, logicalPath string, _ string, _ []byte) (string, error) {
	root, ok, err := managedUploadTargetRoot(targets, logicalPath)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("logical managed upload is outside configured targets")
	}
	return opaqueManagedStoragePathForRoot(root)
}

func opaqueManagedStoragePathForRoot(root string) (string, error) {
	for i := 0; i < 32; i++ {
		buf := make([]byte, opaqueManagedNameBytes)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate opaque managed storage name: %w", err)
		}
		name := hex.EncodeToString(buf)
		candidate, err := protectedManagedStoragePathForRoot(root, name)
		if err != nil {
			return "", err
		}
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect opaque managed storage candidate: %w", err)
		}
	}
	return "", fmt.Errorf("could not choose an unused opaque managed storage name")
}

func protectedManagedStoragePathForRoot(root, opaqueName string) (string, error) {
	if len(opaqueName) != opaqueManagedNameBytes*2 {
		return "", fmt.Errorf("invalid opaque managed storage name")
	}
	if _, err := hex.DecodeString(opaqueName); err != nil {
		return "", fmt.Errorf("invalid opaque managed storage name")
	}
	rootAbs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed upload target: %w", err)
	}
	rootAbs = filepath.Clean(rootAbs)
	resolvedRoot, ok := resolvedContainmentPath(rootAbs)
	if !ok {
		return "", fmt.Errorf("cannot safely resolve managed upload target")
	}
	shardDir := filepath.Join(rootAbs, protectedManagedNamespace, opaqueName[:2])
	if err := requireResolvedManagedContainment(resolvedRoot, shardDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(shardDir, 0o700); err != nil {
		return "", fmt.Errorf("create protected managed storage shard: %w", err)
	}
	// Resolve again after creation so an existing namespace/shard symlink cannot
	// silently redirect the directory creation outside the configured target.
	if err := requireResolvedManagedContainment(resolvedRoot, shardDir); err != nil {
		return "", err
	}
	return filepath.Join(shardDir, opaqueName), nil
}

func requireResolvedManagedContainment(resolvedRoot, path string) error {
	resolvedPath, ok := resolvedContainmentPath(path)
	if !ok {
		return fmt.Errorf("cannot safely resolve protected managed storage path")
	}
	if !pathContainsOrEquals(resolvedRoot, resolvedPath) {
		return fmt.Errorf("protected managed storage path escapes managed upload target")
	}
	return nil
}

// IsProtectedManagedStoragePath reports whether path has the exact reserved
// namespace shape for any configured upload target.
func IsProtectedManagedStoragePath(targets []UploadTarget, path string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	pathAbs = filepath.Clean(pathAbs)
	resolvedPath, pathResolved := resolvedContainmentPath(pathAbs)
	for _, target := range targets {
		if strings.TrimSpace(target.Path) == "" {
			continue
		}
		root, err := filepath.Abs(target.Path)
		if err != nil {
			continue
		}
		root = filepath.Clean(root)
		rel, err := filepath.Rel(filepath.Join(root, protectedManagedNamespace), pathAbs)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != opaqueManagedNameBytes*2 || parts[0] != parts[1][:2] {
			continue
		}
		if _, err := hex.DecodeString(parts[1]); err != nil {
			continue
		}
		resolvedRoot, rootResolved := resolvedContainmentPath(root)
		if pathResolved && rootResolved && pathContainsOrEquals(resolvedRoot, resolvedPath) {
			return true
		}
	}
	return false
}

// CleanupProtectedManagedStorageOrphans removes unreferenced opaque files from
// the reserved namespace. Because ordinary user files never belong in this
// namespace, a startup after a process death between protected upload move and
// DB commit can safely reclaim the stranded ciphertext. Unexpected namespace
// shapes fail closed rather than being deleted speculatively.
func CleanupProtectedManagedStorageOrphans(targets []UploadTarget, referenced []string) error {
	keep := make(map[string]struct{}, len(referenced))
	for _, path := range referenced {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("normalize referenced protected storage path: %w", err)
		}
		keep[filepath.Clean(abs)] = struct{}{}
	}
	for _, target := range targets {
		if strings.TrimSpace(target.Path) == "" {
			continue
		}
		root, err := filepath.Abs(target.Path)
		if err != nil {
			return fmt.Errorf("resolve managed upload target: %w", err)
		}
		namespaceRoot := filepath.Join(filepath.Clean(root), protectedManagedNamespace)
		if _, err := os.Lstat(namespaceRoot); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect protected managed storage namespace: %w", err)
		}
		var emptyShards []string
		err = filepath.WalkDir(namespaceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == namespaceRoot {
				if !entry.IsDir() {
					return fmt.Errorf("protected managed storage namespace is not a directory")
				}
				return nil
			}
			rel, err := filepath.Rel(namespaceRoot, path)
			if err != nil {
				return err
			}
			parts := strings.Split(rel, string(filepath.Separator))
			if entry.IsDir() {
				if len(parts) != 1 || len(parts[0]) != 2 {
					return fmt.Errorf("unexpected directory in protected managed storage namespace")
				}
				if _, err := hex.DecodeString(parts[0]); err != nil {
					return fmt.Errorf("invalid protected managed storage shard")
				}
				emptyShards = append(emptyShards, path)
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return fmt.Errorf("unexpected non-regular entry in protected managed storage namespace")
			}
			if len(parts) != 2 || len(parts[1]) != opaqueManagedNameBytes*2 || parts[0] != parts[1][:2] {
				return fmt.Errorf("invalid protected managed storage entry")
			}
			if _, err := hex.DecodeString(parts[1]); err != nil {
				return fmt.Errorf("invalid protected managed storage entry")
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if _, ok := keep[filepath.Clean(abs)]; ok {
				return nil
			}
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove orphaned protected managed storage entry: %w", err)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("reconcile protected managed storage namespace: %w", err)
		}
		for i := len(emptyShards) - 1; i >= 0; i-- {
			_ = os.Remove(emptyShards[i])
		}
	}
	return nil
}

func managedUploadTargetRoot(targets []UploadTarget, path string) (string, bool, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("resolve logical managed upload path: %w", err)
	}
	pathAbs = filepath.Clean(pathAbs)
	resolvedPath, ok := resolvedContainmentPath(pathAbs)
	if !ok {
		return "", false, fmt.Errorf("cannot safely resolve logical managed upload path")
	}
	best := ""
	for _, target := range targets {
		if strings.TrimSpace(target.Path) == "" {
			continue
		}
		root, err := filepath.Abs(target.Path)
		if err != nil {
			return "", false, fmt.Errorf("resolve managed upload target: %w", err)
		}
		root = filepath.Clean(root)
		resolvedRoot, ok := resolvedContainmentPath(root)
		if !ok {
			return "", false, fmt.Errorf("cannot safely resolve managed upload target")
		}
		if !pathContainsOrEquals(resolvedRoot, resolvedPath) {
			continue
		}
		if len(root) > len(best) {
			best = root
		}
	}
	return best, best != "", nil
}

func moveManagedFileToOpaqueStorage(logicalPath, contentHash string, key []byte) (string, error) {
	physicalPath, err := OpaqueManagedStoragePath(logicalPath, contentHash, key)
	if err != nil {
		return "", err
	}
	if err := os.Rename(logicalPath, physicalPath); err != nil {
		return "", fmt.Errorf("move managed upload to opaque storage: %w", err)
	}
	return physicalPath, nil
}
