package serve

import (
	"os"
	"path/filepath"
	"strings"
)

// resolvedContainmentPath resolves every existing path component, then appends
// any non-existent suffix unchanged. This keeps managed-path checks useful for
// upload destinations that do not exist yet while preventing existing parent
// symlinks from escaping a configured upload root.
func resolvedContainmentPath(path string) (string, bool) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", false
	}

	current := path
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), true
		}
		if !os.IsNotExist(err) {
			return "", false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

// IsManagedUploadPath reports whether path belongs to a configured upload
// target. Protected mode only owns at-rest encryption for files managed by
// Gooru's upload roots; arbitrary indexed library media must not be rewritten.
// Existing symlinks are resolved on both sides of the containment check so a
// symlink nested below an upload root cannot redirect management outside it.
func IsManagedUploadPath(targets []UploadTarget, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	resolvedPath, ok := resolvedContainmentPath(path)
	if !ok {
		return false
	}
	for _, target := range targets {
		root := strings.TrimSpace(target.Path)
		if root == "" {
			continue
		}
		resolvedRoot, ok := resolvedContainmentPath(root)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(resolvedRoot, resolvedPath)
		if err != nil || rel == "." {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
