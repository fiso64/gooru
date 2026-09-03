package serve

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gooru.local/internal/encryptedfile"
)

type mediaReadSource interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

type openedMediaSource struct {
	mediaReadSource
	size    int64
	modTime time.Time
}

func (m *MediaService) openMediaSource(path string) (*openedMediaSource, error) {
	if m.cfg.Encryption.Enabled {
		encrypted, err := encryptedfile.IsEncryptedFile(path)
		if err != nil {
			return nil, err
		}
		if encrypted {
			file, err := encryptedfile.Open(path, m.cfg.Encryption.Key)
			if err != nil {
				return nil, err
			}
			return &openedMediaSource{
				mediaReadSource: file,
				size:            file.Size(),
				modTime:         file.ModTime(),
			}, nil
		}
		if IsManagedUploadPath(m.cfg.Uploads.Targets, path) {
			return nil, fmt.Errorf("protected upload is not encrypted: %w", encryptedfile.ErrInvalidFormat)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, fmt.Errorf("media source is a directory")
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("media source is not a regular file")
	}
	return &openedMediaSource{
		mediaReadSource: file,
		size:            info.Size(),
		modTime:         info.ModTime(),
	}, nil
}
