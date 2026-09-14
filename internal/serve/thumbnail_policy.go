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

// thumbnailQualityGenerationPolicy applies an explicit encoder quality while
// keeping the same storage boundary as ordinary thumbnail generation. The bool
// reports whether the selected backend supports the quality override; callers
// can preserve the ordinary thumbnail fallback when it does not.
type thumbnailQualityGenerationPolicy func(*MediaService, types.FileInfo, io.Writer, int, string, int) (bool, error)

type videoPathThumbnailer interface {
	ThumbnailVideoPath(src string, dst io.Writer, size int, format string) error
}

func newThumbnailGenerationPolicy(protected bool) thumbnailGenerationPolicy {
	if protected {
		return generateThumbnailFromLogicalSource
	}
	return generateThumbnailFromPath
}

func newThumbnailQualityGenerationPolicy(protected bool) thumbnailQualityGenerationPolicy {
	if protected {
		return generateThumbnailQualityFromLogicalSource
	}
	return generateThumbnailQualityFromPath
}

func generateThumbnailFromPath(m *MediaService, file types.FileInfo, dst io.Writer, size int, format string) error {
	if _, ok := thumbnailSupportForPath(file.Path); !ok {
		return unsupportedThumbnailCapability(file.Path)
	}
	return m.thumbnailer.Thumbnail(file.Path, dst, size, format)
}

func generateThumbnailQualityFromPath(m *MediaService, file types.FileInfo, dst io.Writer, size int, format string, quality int) (bool, error) {
	qualityThumbnailer, ok := m.thumbnailer.(QualityThumbnailer)
	if !ok {
		return false, nil
	}
	return true, qualityThumbnailer.ThumbnailQuality(file.Path, dst, size, format, quality)
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
		// Every declared video container reaches the same ffmpeg pathname backend.
		// Protected mode differs only in how that seekable logical location is
		// supplied: the authenticated random-access reader is exposed through a
		// short-lived range-capable loopback URL, preserving #466's bounded chunk
		// working set without plaintext disk materialization.
		if pathThumbnailer, ok := m.thumbnailer.(videoPathThumbnailer); ok {
			path, cleanup, available, err := logicalVideoSeekablePath(file.Path, source, source.size)
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
		// Images and GIFs are naturally source-driven and do not require a seekable
		// external location for their backend.
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

func generateThumbnailQualityFromLogicalSource(m *MediaService, file types.FileInfo, dst io.Writer, size int, format string, quality int) (bool, error) {
	sourceThumbnailer, ok := m.thumbnailer.(SourceQualityThumbnailer)
	if !ok {
		return false, nil
	}
	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		return true, err
	}
	defer source.Close()
	return true, sourceThumbnailer.ThumbnailSourceQuality(file.Path, source, dst, size, format, quality)
}

func unsupportedThumbnailCapability(path string) error {
	return &UnsupportedMediaError{
		Backend: "media",
		Reason:  "thumbnail support for " + mediaKindForType(mediaTypeForPath(path)) + " does not declare protected-mode access",
		Err:     ErrUnsupportedMedia,
	}
}
