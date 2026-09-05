package serve

import (
	"io"
	"time"
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
	if m.sourceResolverErr != nil {
		return nil, m.sourceResolverErr
	}
	source, err := m.sourceResolver.Open(path)
	if err != nil {
		return nil, err
	}
	return &openedMediaSource{
		mediaReadSource: source,
		size:            source.Size(),
		modTime:         source.ModTime(),
	}, nil
}
