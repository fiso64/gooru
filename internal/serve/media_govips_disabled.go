//go:build !govips

package serve

import "io"

type govipsDisabledThumbnailer struct{}

func newPrimaryImageThumbnailer() Thumbnailer {
	return govipsDisabledThumbnailer{}
}

func (govipsDisabledThumbnailer) BackendVersion() string {
	return "govips:disabled"
}

func (govipsDisabledThumbnailer) Thumbnail(string, io.Writer, int, string) error {
	return &UnsupportedMediaError{
		Backend: "govips",
		Reason:  "govips support is not compiled in; rebuild with -tags govips to enable libvips thumbnails",
		Err:     ErrUnsupportedMedia,
	}
}

func thumbnailImageSourcePrimary(string, io.ReadSeeker, io.Writer, int, string, int) error {
	return &UnsupportedMediaError{
		Backend: "govips",
		Reason:  "govips support is not compiled in; rebuild with -tags govips to enable libvips thumbnails",
		Err:     ErrUnsupportedMedia,
	}
}
