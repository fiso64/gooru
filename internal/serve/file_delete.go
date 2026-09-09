package serve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/types"
)

var ErrFileNotManaged = errors.New("file is outside configured upload targets")

type stagedFileDeletion struct {
	originalPath string
	stagedPath   string
	stagingDir   string
}

func (s *Server) canDeleteFilePath(path string) bool {
	_, ok := s.managedDeleteCandidatePath(path)
	return ok
}

func (s *Server) managedDeletePath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	filePath, err := canonicalExistingPath(path)
	if err != nil {
		return "", false
	}
	return s.matchManagedUploadPath(filePath)
}

// managedDeleteCandidatePath validates a tracked path even when the file itself
// has already disappeared from disk. The containing directory must still exist
// and is canonicalized so a symlink cannot turn an apparently managed missing
// path into a deletion outside the configured upload target.
func (s *Server) managedDeleteCandidatePath(path string) (string, bool) {
	if managedPath, ok := s.managedDeletePath(path); ok {
		return managedPath, true
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	// The fallback below is deliberately only for a genuinely absent final path.
	// Existing symlinks (or other unusual file types/errors) must retain the stricter
	// existing-path validation above rather than being reinterpreted as missing files.
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	parent, err := canonicalExistingPath(filepath.Dir(absolute))
	if err != nil {
		return "", false
	}
	candidate := filepath.Join(parent, filepath.Base(absolute))
	return s.matchManagedUploadPath(candidate)
}

func (s *Server) matchManagedUploadPath(filePath string) (string, bool) {
	for _, target := range s.cfg.Uploads.Targets {
		root := strings.TrimSpace(target.Path)
		if root == "" {
			continue
		}
		rootPath, err := canonicalExistingPath(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootPath, filePath)
		if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return filepath.Clean(filePath), true
	}
	return "", false
}

func canonicalExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func (s *Server) deleteManagedFile(ctx context.Context, publicID string) (bool, error) {
	file, err := s.getFileByPublicID(ctx, publicID)
	if err != nil {
		return false, err
	}
	managedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))
	if !ok {
		return false, ErrFileNotManaged
	}
	file.Path = managedPath

	staged, err := stageFileDeletion(file)
	if err != nil {
		return false, err
	}

	deleted, deleteErr := s.deleteFileByPublicID(ctx, publicID)
	if deleteErr != nil || !deleted {
		if staged != nil {
			if rollbackErr := staged.rollback(); rollbackErr != nil {
				return false, fmt.Errorf("delete library location: %v; restore file: %w", deleteErr, rollbackErr)
			}
		}
		if deleteErr != nil {
			return false, deleteErr
		}
		return false, ErrNotFound
	}

	if staged != nil {
		if err := staged.commit(); err != nil {
			if rollbackErr := staged.rollback(); rollbackErr != nil {
				return false, fmt.Errorf("remove staged file: %v; restore file: %w", err, rollbackErr)
			}
			return false, fmt.Errorf("remove staged file: %w", err)
		}
	}
	return true, nil
}

func stageFileDeletion(file types.FileInfo) (*stagedFileDeletion, error) {
	stagingDir, err := os.MkdirTemp(filepath.Dir(file.Path), ".gooru-delete-")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("prepare file deletion: %w", err)
	}
	staged := &stagedFileDeletion{
		originalPath: file.Path,
		stagedPath:   filepath.Join(stagingDir, filepath.Base(file.Path)),
		stagingDir:   stagingDir,
	}
	if err := os.Rename(file.Path, staged.stagedPath); err != nil {
		_ = os.Remove(stagingDir)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stage file deletion: %w", err)
	}
	return staged, nil
}

func (d *stagedFileDeletion) rollback() error {
	if err := os.Rename(d.stagedPath, d.originalPath); err != nil {
		return err
	}
	_ = os.Remove(d.stagingDir)
	return nil
}

func (d *stagedFileDeletion) commit() error {
	if err := os.Remove(d.stagedPath); err != nil {
		return err
	}
	_ = os.Remove(d.stagingDir)
	return nil
}
