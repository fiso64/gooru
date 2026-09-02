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
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	filePath, err := canonicalExistingPath(path)
	if err != nil {
		return false
	}
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
		return true
	}
	return false
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
	if !s.canDeleteFilePath(file.Path) {
		return false, ErrFileNotManaged
	}

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
