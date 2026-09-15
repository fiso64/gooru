package serve

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"gooru.local/types"
)

// serveOriginal streams original tracked content after resolving its logical
// storage source. A missing backing file remains a 404, while resolver,
// permission, encryption, and other storage failures are service failures.
func (m *MediaService) serveOriginal(w http.ResponseWriter, r *http.Request, file types.FileInfo, download bool) {
	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not_found", "file content not found", nil)
			return
		}
		slog.Error("failed to open original media source", "error", err)
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file content is unavailable", nil)
		return
	}
	defer source.Close()

	if download {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(file.Path)))
	}
	if isOriginalDocumentNavigation(r) {
		applyOriginalDocumentContentPolicy(w, file, source)
	} else {
		applyOriginalContentPolicy(w, file)
	}
	http.ServeContent(w, r, filepath.Base(file.Path), source.modTime, source)
}
