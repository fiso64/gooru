package serve

import (
	"io"
	"net/http"

	"gooru.local/types"
)

// Eligibility is checked using the logical source, not a raw filesystem path.
func (m *MediaService) losslessJPEGAvailable(file types.FileInfo) bool {
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
