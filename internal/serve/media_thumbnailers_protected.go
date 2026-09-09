package serve

import "io"

// ThumbnailVideoPath bypasses media-kind dispatch because protected mode may
// expose decrypted video bytes through a short-lived seekable location whose
// basename is not the logical filename. The logical file was already classified
// as video before this method is used.
func (t *MediaThumbnailer) ThumbnailVideoPath(src string, dst io.Writer, size int, format string) error {
	if t == nil || t.video == nil {
		return &UnsupportedMediaError{Backend: "ffmpeg", Reason: "video thumbnail backend is not configured", Err: ErrUnsupportedMedia}
	}
	return t.video.Thumbnail(src, dst, size, format)
}
