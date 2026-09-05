package serve

import (
	"errors"
	"io"
	"os"

	"gooru.local/internal/managedfile"
)

func (s *Server) persistUploadedFile(dst *os.File, src io.Reader) (int64, error) {
	size, err := s.managedFiles.Write(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
	if errors.Is(err, managedfile.ErrTooLarge) {
		return size, errUploadTooLarge
	}
	return size, err
}
