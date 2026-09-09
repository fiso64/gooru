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

type protectedVideoPathThumbnailer interface {
	ThumbnailVideoPath(src string, dst io.Writer, size int, format string) error
}

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
	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		return err
	}
	defer source.Close()

	// ffmpeg can consume simple videos from stdin, but real MP4 files commonly
	// require seeking (for example when the moov atom lives near the end). Prefer
	// a short-lived loopback range source when available so protected mode keeps
	// seek semantics without writing plaintext to disk or buffering whole videos.
	if mediaKindForType(mediaTypeForPath(file.Path)) == "video" {
		if pathThumbnailer, ok := m.thumbnailer.(protectedVideoPathThumbnailer); ok {
			path, cleanup, available, err := protectedVideoSeekablePath(file.Path, source)
			if err != nil {
				return err
			}
			if available {
				pathErr := pathThumbnailer.ThumbnailVideoPath(path, dst, size, format)
				cleanup()
				if pathErr == nil {
					return nil
				}
				if _, err := source.Seek(0, io.SeekStart); err != nil {
					return pathErr
				}
			}
		}
	}

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
