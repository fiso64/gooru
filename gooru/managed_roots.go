package gooru

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gooru.local/types"
)

const managedRootMetaPrefix = "managed_upload_root:"

// ReconcileManagedRoots remembers the filesystem root associated with each
// stable managed-upload target ID and rebases tracked locations atomically
// when that root changes. The range predicate keeps a million-file move on
// the indexed locations.path column instead of walking every row in Go.
func (c *Client) ReconcileManagedRoots(roots map[string]string) (int, error) {
	ids := make([]string, 0, len(roots))
	normalized := make(map[string]string, len(roots))
	for id, root := range roots {
		id = strings.TrimSpace(id)
		root = strings.TrimSpace(root)
		if id == "" || root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return 0, fmt.Errorf("resolve managed root %q: %w", id, err)
		}
		ids = append(ids, id)
		normalized[id] = filepath.Clean(abs)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return 0, nil
	}

	tx, err := c.store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	moved := 0
	for _, id := range ids {
		key := managedRootMetaPrefix + id
		newRoot := normalized[id]
		var oldRoot string
		err := tx.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&oldRoot)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec("INSERT INTO meta (key, value) VALUES (?, ?)", key, newRoot); err != nil {
				return 0, fmt.Errorf("remember managed root %q: %w", id, err)
			}
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("read managed root %q: %w", id, err)
		}
		oldRoot = filepath.Clean(oldRoot)
		if oldRoot == newRoot {
			continue
		}

		count, err := rebaseManagedRootTx(tx, oldRoot, newRoot)
		if err != nil {
			return 0, fmt.Errorf("rebase managed root %q: %w", id, err)
		}
		if _, err := tx.Exec("UPDATE meta SET value = ? WHERE key = ?", newRoot, key); err != nil {
			return 0, fmt.Errorf("update managed root %q: %w", id, err)
		}
		moved += count
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return moved, nil
}

func rebaseManagedRootTx(tx interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}, oldRoot, newRoot string) (int, error) {
	if oldRoot == newRoot {
		return 0, nil
	}
	sep := string(os.PathSeparator)
	prefix := oldRoot
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	// filepath separators are single-byte ASCII on supported platforms. The
	// immediate successor creates an exact indexed lexical prefix range.
	upper := prefix[:len(prefix)-1] + string(os.PathSeparator+1)
	res, err := tx.Exec(`
		UPDATE locations
		SET path = CASE
			WHEN path = ? THEN ?
			ELSE ? || substr(path, length(?) + 1)
		END
		WHERE path = ? OR (path >= ? AND path < ?)`,
		oldRoot, newRoot, newRoot, oldRoot, oldRoot, prefix, upper)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		UPDATE managed_storage_locations
		SET physical_path = CASE
			WHEN physical_path = ? THEN ?
			ELSE ? || substr(physical_path, length(?) + 1)
		END
		WHERE physical_path = ? OR (physical_path >= ? AND physical_path < ?)`,
		oldRoot, newRoot, newRoot, oldRoot, oldRoot, prefix, upper); err != nil {
		return 0, err
	}
	return int(n), nil
}

// ManagedStoragePath returns the physical storage path for a managed location.
// Absence of a row means the logical location path is also the physical path.
func (c *Client) ManagedStoragePath(locationID int64) (string, bool, error) {
	var path string
	err := c.store.QueryRow(`SELECT physical_path FROM managed_storage_locations WHERE location_id = ?`, locationID).Scan(&path)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

// SetManagedStoragePath records the opaque physical path for a managed
// location while leaving the canonical logical path in locations.path.
func (c *Client) SetManagedStoragePath(locationID int64, physicalPath string) error {
	physicalPath = strings.TrimSpace(physicalPath)
	if locationID <= 0 {
		return fmt.Errorf("invalid managed storage location id %d", locationID)
	}
	if physicalPath == "" {
		return fmt.Errorf("managed storage path is empty")
	}
	_, err := c.store.Exec(`
		INSERT INTO managed_storage_locations (location_id, physical_path)
		VALUES (?, ?)
		ON CONFLICT(location_id) DO UPDATE SET physical_path = excluded.physical_path`,
		locationID, physicalPath)
	return err
}

// ClearManagedStoragePath removes an opaque physical-path mapping after a
// managed file has been restored to its canonical logical path.
func (c *Client) ClearManagedStoragePath(locationID int64) error {
	_, err := c.store.Exec(`DELETE FROM managed_storage_locations WHERE location_id = ?`, locationID)
	return err
}

// ResolveManagedStorage attaches the physical storage path to a tracked file
// without changing its canonical logical path or filename metadata.
func (c *Client) ResolveManagedStorage(file types.FileInfo) (types.FileInfo, error) {
	path, ok, err := c.ManagedStoragePath(file.ID)
	if err != nil {
		return file, err
	}
	if ok {
		file.StoragePath = path
	} else {
		file.StoragePath = ""
	}
	return file, nil
}

// RecoverMissingManagedPath safely heals a pre-metadata library that was
// already moved. Uploads are stored directly under target roots, so a stale
// basename can identify candidates; content hashing is the authority and an
// ambiguous match is deliberately left untouched.
func (c *Client) RecoverMissingManagedPath(file types.FileInfo, roots []string) (types.FileInfo, bool, error) {
	if _, err := os.Lstat(file.Path); err == nil || !os.IsNotExist(err) {
		return file, false, nil
	}

	matches := make([]string, 0, 1)
	seen := make(map[string]struct{})
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		candidate := filepath.Join(filepath.Clean(abs), filepath.Base(file.Path))
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if _, err := os.Lstat(candidate); err != nil {
			continue
		}
		hash, err := c.hasher.HashFile(candidate)
		if err != nil || hash != file.Hash {
			continue
		}
		matches = append(matches, candidate)
		if len(matches) > 1 {
			return file, false, nil
		}
	}
	if len(matches) != 1 {
		return file, false, nil
	}

	candidate := matches[0]
	meta, err := c.hasher.FileMetadata(candidate)
	if err != nil {
		return file, false, err
	}
	tx, err := c.store.Begin()
	if err != nil {
		return file, false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE locations
		SET path = ?, size_bytes = ?, mod_time = ?, extension = ?
		WHERE id = ? AND path = ? AND content_hash = ?`,
		candidate, meta.Size, meta.ModTime.Unix(), filepath.Ext(candidate), file.ID, file.Path, file.Hash)
	if err != nil {
		return file, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return file, false, err
	}
	if n != 1 {
		return file, false, nil
	}
	if err := tx.Commit(); err != nil {
		return file, false, err
	}
	repaired, err := c.GetFileInfoByLocationID(file.ID)
	if err != nil {
		return file, false, err
	}
	return repaired, true, nil
}
