package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CleanupProtectedMediaCache removes only Gooru's owned encrypted derivative
// namespace when protected storage is being disabled. Derivatives are
// disposable and ordinary mode will regenerate plaintext entries lazily.
func CleanupProtectedMediaCache(cfg MediaConfig) error {
	root, err := mediaCacheRoot(cfg)
	if err != nil {
		return err
	}
	protectedRoot := filepath.Join(root, encryptedDerivativeNamespace)
	info, err := os.Lstat(protectedRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect protected media cache: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(protectedRoot); err != nil {
			return fmt.Errorf("remove protected media cache symlink: %w", err)
		}
		return nil
	}
	if !info.IsDir() {
		return fmt.Errorf("protected media cache namespace is not a directory: %s", protectedRoot)
	}
	if err := os.RemoveAll(protectedRoot); err != nil {
		return fmt.Errorf("remove protected media cache: %w", err)
	}
	return nil
}
