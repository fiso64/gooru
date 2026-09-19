package serve

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
)

// encodeResizedImage accepts a logical image stream, leaving acquisition and
// lifetime management to its caller. Archive entries and standalone files can
// therefore share the same bounded decode, scaling, and output policy.
func encodeResizedImage(src io.Reader, dst io.Writer, size int, format string, quality int) error {
	img, _, err := image.Decode(src)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupportedMedia, err)
	}
	resized := scaleImage(img, size)
	switch format {
	case "jpeg":
		return jpeg.Encode(dst, resized, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(dst, resized)
	default:
		return fmt.Errorf("%w: thumbnail format %q", ErrUnsupportedMedia, format)
	}
}
