package serve

import "io"

// ThumbnailVideoPath runs the shared video backend against a seekable location
// whose URL/path may not preserve the tracked file's logical basename. Clear
// mode supplies the ordinary pathname through Thumbnail; protected mode supplies
// the equivalent short-lived logical seekable source through this entry point.
func (t *MediaThumbnailer) ThumbnailVideoPath(src string, dst io.Writer, size int, format string) error {
	if t == nil || t.video == nil {
		return &UnsupportedMediaError{Backend: "ffmpeg", Reason: "video thumbnail backend is not configured", Err: ErrUnsupportedMedia}
	}
	return t.video.Thumbnail(src, dst, size, format)
}
