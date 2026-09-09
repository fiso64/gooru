package serve

import (
	"path/filepath"
	"strings"
)

type protectedThumbnailAccess string

const (
	protectedThumbnailSource        protectedThumbnailAccess = "logical-source"
	protectedThumbnailSeekableVideo protectedThumbnailAccess = "seekable-video"
	protectedThumbnailComicArchive  protectedThumbnailAccess = "comic-archive"
)

type thumbnailSupport struct {
	kind            string
	protectedAccess protectedThumbnailAccess
}

// thumbnailSupportForPath is the capability boundary for thumbnail support.
// Every media kind that the thumbnail stack accepts must declare how protected
// mode can access it here. This intentionally makes adding a new thumbnailable
// file kind a two-sided decision: clear-path support and protected-source
// support cannot drift apart unnoticed.
func thumbnailSupportForPath(path string) (thumbnailSupport, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".cbz" {
		return thumbnailSupport{kind: "comic", protectedAccess: protectedThumbnailComicArchive}, true
	}

	kind := mediaKindForType(mediaTypeForPath(path))
	switch kind {
	case "photo", "gif":
		return thumbnailSupport{kind: kind, protectedAccess: protectedThumbnailSource}, true
	case "video":
		// All video containers use the same seek-capable ffmpeg contract. Clear
		// mode supplies the ordinary pathname; protected mode supplies a short-lived
		// seekable logical source backed by authenticated random access. Keeping the
		// transport choice at the media-kind boundary prevents container-specific
		// protected routing from drifting away from clear-mode support.
		return thumbnailSupport{kind: kind, protectedAccess: protectedThumbnailSeekableVideo}, true
	default:
		return thumbnailSupport{}, false
	}
}
