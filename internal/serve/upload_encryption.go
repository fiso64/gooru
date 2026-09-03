package serve

import (
	"io"
	"os"

	"gooru.local/internal/encryptedfile"
)

func (s *Server) persistUploadedFile(dst *os.File, src io.Reader) (int64, error) {
	if !s.cfg.Encryption.Enabled {
		return copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
	}
	if s.cfg.Uploads.MaxFileSizeBytes <= 0 {
		return encryptedfile.EncryptStream(dst, src, s.cfg.Encryption.Key)
	}
	limited := &io.LimitedReader{R: src, N: s.cfg.Uploads.MaxFileSizeBytes + 1}
	size, err := encryptedfile.EncryptStream(dst, limited, s.cfg.Encryption.Key)
	if err != nil {
		return size, err
	}
	if size > s.cfg.Uploads.MaxFileSizeBytes {
		return size, errUploadTooLarge
	}
	return size, nil
}
