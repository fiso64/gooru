package serve

import (
	"io"
	"net/http"

	"gooru.local/types"
)

// Eligibility is checked using the logical source, not a raw filesystem path.
func (m *MediaService) losslessJPEGAvailable(file types.FileInfo) bool {
	if m == nil { return false }
	if !m.cfg.Media.LosslessJPEGTranscode || m.losslessJPEGTool == "" ||
		file.Hash == "" || mediaTypeForPath(file.Path) != "image/jpeg" {
		return false
	}
	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		return false
	}
	defer source.Close()
	progressive, err := jpegFrameIsProgressive(source)
	return err == nil && progressive
}

func (m *MediaService) ServeLosslessJPEG(w http.ResponseWriter, r *http.Request, file types.FileInfo) {
	if !m.losslessJPEGAvailable(file) {
		writeError(w, http.StatusNotFound, "not_found", "lossless derivative is unavailable", nil)
		return
	}
	if m.derivativeStoreErr != nil || m.derivatives == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
		return
	}
	relative := m.derivativeRelativePath(file, "lossless", 0, "jpeg")
	artifact, err := m.derivatives.GetOrGenerate(relative, func(dst io.Writer) error {
		source, err := m.openMediaSource(fileStoragePath(file))
		if err != nil {
			return err
		}
		defer source.Close()
		return transcodeProgressiveJPEG(r.Context(), source, dst, m.losslessJPEGTool)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate lossless derivative", nil)
		return
	}
	defer artifact.Close()
	w.Header().Set("X-Gooru-Cache", artifact.CacheStatus)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", artifact.CacheControl)
	if artifact.CacheControl == "private, no-store" {
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}
	http.ServeContent(w, r, artifact.Name, artifact.ModTime, artifact.Reader)
}
