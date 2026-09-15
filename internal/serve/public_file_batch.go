package serve

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"gooru.local/types"
)

// GetFilesByPublicIDs resolves explicit durable-mutation targets without the
// per-ID database lookup chain. Current managed-storage mappings are returned
// by the bulk query; only legacy pre-metadata rows whose logical path has gone
// missing need the slower recovery path.
func (l *GooruLibrary) GetFilesByPublicIDs(ctx context.Context, publicIDs []string) ([]types.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files, err := l.client.GetFileInfosByPublicIDs(publicIDs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(l.managedRoots) == 0 {
		return files, nil
	}
	for i := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if files[i].StoragePath != "" {
			continue
		}
		if _, err := os.Lstat(files[i].Path); err == nil || !os.IsNotExist(err) {
			continue
		}
		repaired, ok, err := l.client.RecoverMissingManagedPath(files[i], l.managedRoots)
		if err != nil {
			return nil, err
		}
		if ok {
			files[i] = repaired
		}
	}
	return files, nil
}