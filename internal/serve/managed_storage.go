package serve

import (
	"strings"

	"gooru.local/types"
)

// fileStoragePath resolves the physical path backing a tracked file while
// keeping file.Path as the canonical logical path exposed to feature code.
func fileStoragePath(file types.FileInfo) string {
	if path := strings.TrimSpace(file.StoragePath); path != "" {
		return path
	}
	return file.Path
}
