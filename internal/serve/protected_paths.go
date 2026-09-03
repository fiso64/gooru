package serve

import (
	"path/filepath"
	"strings"
)

// IsManagedUploadPath reports whether path belongs to a configured upload
// target. Protected mode only owns at-rest encryption for files managed by
// Gooru's upload roots; arbitrary indexed library media must not be rewritten.
func IsManagedUploadPath(targets []UploadTarget, path string) bool {
	if path == "" {
		return false
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return false
	}
	for _, target := range targets {
		root := filepath.Clean(strings.TrimSpace(target.Path))
		if root == "." || root == "" || !filepath.IsAbs(root) {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
