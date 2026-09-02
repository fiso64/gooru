package serve

import (
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
	return &openedMediaSource{
		mediaReadSource: file,
		size:            info.Size(),
		modTime:         info.ModTime(),
	}, nil
}
