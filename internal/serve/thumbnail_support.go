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
		// WebM is designed for streaming and ffmpeg can consume it directly from
		// the authenticated logical source. Prefer that path in protected mode so
		// thumbnailing does not depend on loopback range/seek behavior. MP4 and
		// other seek-oriented containers retain the short-lived seekable source.
		if ext == ".webm" {
			return thumbnailSupport{kind: kind, protectedAccess: protectedThumbnailSource}, true
		}
		return thumbnailSupport{kind: kind, protectedAccess: protectedThumbnailSeekableVideo}, true
	default:
		return thumbnailSupport{}, false
	}
}
