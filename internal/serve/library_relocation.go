package serve

import (
	"os"
	"path/filepath"
	"strings"

	"gooru.local/types"
)

func (l *GooruLibrary) configureManagedUploadRoots(targets []UploadTarget) error {
	l.managedTargets = append([]UploadTarget(nil), targets...)
	byID := make(map[string]string, len(targets))
	roots := make([]string, 0, len(targets))
	for _, target := range targets {
		id := strings.TrimSpace(target.ID)
		path := strings.TrimSpace(target.Path)
		if id == "" || path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		abs = filepath.Clean(abs)
		byID[id] = abs
		roots = append(roots, abs)
	}
	l.managedRoots = roots
	_, err := l.client.ReconcileManagedRoots(byID)
	return err
}

func (l *GooruLibrary) repairManagedPath(file types.FileInfo) (types.FileInfo, error) {
	resolved, err := l.client.ResolveManagedStorage(file)
	if err != nil {
		return file, err
	}
	if resolved.StoragePath != "" {
		return resolved, nil
	}
	file = resolved
	if len(l.managedRoots) == 0 {
		return file, nil
	}
	if _, err := os.Lstat(file.Path); err == nil || !os.IsNotExist(err) {
		return file, nil
	}
	repaired, ok, err := l.client.RecoverMissingManagedPath(file, l.managedRoots)
	if err != nil || !ok {
		return file, err
	}
	return repaired, nil
}
