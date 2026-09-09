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
	if _, ok := thumbnailSupportForPath(file.Path); !ok {
		return unsupportedThumbnailCapability(file.Path)
	}
	return m.thumbnailer.Thumbnail(file.Path, dst, size, format)
}

func generateThumbnailFromLogicalSource(m *MediaService, file types.FileInfo, dst io.Writer, size int, format string) error {
	support, ok := thumbnailSupportForPath(file.Path)
	if !ok {
		return unsupportedThumbnailCapability(file.Path)
	}
	if support.protectedAccess == protectedThumbnailComicArchive {
		return m.thumbnailCBZFirstPageFile(file, dst, size, format)
	}

	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		return err
	}
	defer source.Close()

	switch support.protectedAccess {
	case protectedThumbnailSeekableVideo:
		// ffmpeg can consume simple videos from stdin, but real MP4 files commonly
		// require seeking (for example when the moov atom lives near the end). Prefer
		// a short-lived loopback range source when available so protected mode keeps
		// seek semantics without writing plaintext to disk or buffering whole videos.
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
	case protectedThumbnailSource:
		// The generic protected path below is the declared strategy for images and
		// GIFs. Keeping the declaration separate from backend dispatch makes new
		// supported media kinds opt in to protected-mode semantics explicitly.
	default:
		return unsupportedThumbnailCapability(file.Path)
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

func unsupportedThumbnailCapability(path string) error {
	return &UnsupportedMediaError{
		Backend: "media",
		Reason:  "thumbnail support for " + mediaKindForType(mediaTypeForPath(path)) + " does not declare protected-mode access",
		Err:     ErrUnsupportedMedia,
	}
}
