package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CleanupPlaintextMediaCache removes Gooru derivative files written by ordinary
// mode before protected mode begins serving. It deliberately recognizes only
// the cache layout Gooru owns rather than recursively deleting an arbitrary
// configured cache directory.
func CleanupPlaintextMediaCache(cfg MediaConfig) error {
	root, err := mediaCacheRoot(cfg)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read media cache root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !isDerivativeShard(entry.Name()) {
			continue
		}
		shard := filepath.Join(root, entry.Name())
		files, err := os.ReadDir(shard)
		if err != nil {
			return fmt.Errorf("read media cache shard: %w", err)
		}
		for _, file := range files {
			if file.Type()&os.ModeSymlink != 0 || file.IsDir() || !isDerivativeCacheName(file.Name()) {
				continue
			}
			if err := os.Remove(filepath.Join(shard, file.Name())); err != nil {
				return fmt.Errorf("remove plaintext media derivative: %w", err)
			}
		}
		if remaining, err := os.ReadDir(shard); err != nil {
			return fmt.Errorf("recheck media cache shard: %w", err)
		} else if len(remaining) == 0 {
			if err := os.Remove(shard); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove empty media cache shard: %w", err)
			}
		}
	}
	return nil
}

func mediaCacheRoot(cfg MediaConfig) (string, error) {
	if strings.TrimSpace(cfg.CacheDir) != "" {
		return cfg.CacheDir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "gooru", "media-cache"), nil
}

func isDerivativeShard(name string) bool {
	return len(name) == 2 && isLowerHex(name)
}

func isDerivativeCacheName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".jpg" && ext != ".png" {
		return false
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return len(base) == 64 && isLowerHex(base)
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
