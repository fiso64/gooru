package serve

import (
	"io"

	"gooru.local/types"
)

// thumbnailGenerationPolicy is the media composition boundary between feature
// code and storage/backend details. MediaService asks for a derivative of a
// logical tracked file; the selected policy decides whether an optimized
// pathname backend is safe or whether bytes must come from the logical source
// resolver.
type thumbnailGenerationPolicy func(*MediaService, types.FileInfo, io.Writer, int, string) error

func newThumbnailGenerationPolicy(protected bool) thumbnailGenerationPolicy {
	if protected {
		return generateThumbnailFromLogicalSource
	}
	return generateThumbnailFromPath
}

func generateThumbnailFromPath(m *MediaService, file types.FileInfo, dst io.Writer, size int, format string) error {
	return m.thumbnailer.Thumbnail(file.Path, dst, size, format)
}

func generateThumbnailFromLogicalSource(m *MediaService, file types.FileInfo, dst io.Writer, size int, format string) error {
	source, err := m.openMediaSource(file.Path)
	if err != nil {
		return err
	}
	defer source.Close()

	sourceThumbnailer, ok := m.thumbnailer.(SourceThumbnailer)
	if !ok {
		return &UnsupportedMediaError{
			Backend: "media",
			Reason:  "thumbnail backend cannot read logical media sources",
			Err:     ErrUnsupportedMedia,
		}
	}
	return sourceThumbnailer.ThumbnailSource(file.Path, source, dst, size, format)
}
