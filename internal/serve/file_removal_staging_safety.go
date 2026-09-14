package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// validatePersistedDeletionParent re-establishes the managed-path boundary for
// a durable deletion path before a retry or cancellation cleanup touches its
// persisted staging state. The final original path may legitimately be absent
// or occupied by a replacement, so only its containing directory is resolved.
func (s *Server) validatePersistedDeletionParent(originalPath string) error {
	absolute, err := filepath.Abs(filepath.Clean(originalPath))
	if err != nil {
		return fmt.Errorf("resolve persisted file removal path: %w", err)
	}
	parent, err := canonicalExistingPath(filepath.Dir(absolute))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve persisted file removal parent: %w", err)
	}
	candidate := filepath.Join(parent, filepath.Base(absolute))
	managedPath, ok := s.matchManagedUploadPath(candidate)
	if !ok {
		return ErrFileNotManaged
	}
	if filepath.Clean(managedPath) != filepath.Clean(absolute) {
		return errors.New("managed file parent changed after deletion was queued")
	}
	return nil
}

// safeStagedDeletionEntryExists is the durable-deletion equivalent of Lstat:
// the final staging entry must never be a symlink because rollback would move
// that symlink into the tracked original path.
func safeStagedDeletionEntryExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return false, errors.New("file removal staging path is a symlink")
		}
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func validateDeletionStagingDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, errors.New("file removal staging directory is not a real directory")
		}
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func ensureDeletionStagingDirectory(path string) error {
	exists, err := validateDeletionStagingDirectory(path)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("prepare file deletion: %w", err)
	}
	_, err = validateDeletionStagingDirectory(path)
	return err
}
