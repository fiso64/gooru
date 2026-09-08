//go:build govips

package serve

import (
	"fmt"
	"io"
	"sync"

	"github.com/davidbyttow/govips/v2/vips"
)

type GovipsImageThumbnailer struct{}

var (
	govipsStartupOnce sync.Once
	govipsStartupErr  error
)

func newPrimaryImageThumbnailer() Thumbnailer {
	return GovipsImageThumbnailer{}
}

func (GovipsImageThumbnailer) BackendVersion() string {
	if err := ensureGovipsStarted(); err != nil {
		return "govips:unavailable:" + sanitizeVersion(err.Error())
	}
	return "govips:" + sanitizeVersion(vips.Version)
}

func (t GovipsImageThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	return t.ThumbnailQuality(src, dst, size, format, derivativeJPEGQuality)
}

func (GovipsImageThumbnailer) ThumbnailQuality(src string, dst io.Writer, size int, format string, quality int) error {
	if err := ensureGovipsStarted(); err != nil {
		return &UnsupportedMediaError{Backend: "govips", Reason: "govips startup failed", Err: err}
	}
	image, err := vips.NewThumbnailWithSizeFromFile(src, size, size, vips.InterestingNone, vips.SizeDown)
	if err != nil {
		return &UnsupportedMediaError{Backend: "govips", Reason: "failed to load image thumbnail", Err: err}
	}
	defer image.Close()
	return exportGovipsThumbnail(image, dst, format, quality)
}

func thumbnailImageSourcePrimary(_ string, src io.ReadSeeker, dst io.Writer, size int, format string, quality int) error {
	if err := ensureGovipsStarted(); err != nil {
		return &UnsupportedMediaError{Backend: "govips", Reason: "govips startup failed", Err: err}
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	buf, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	image, err := vips.NewThumbnailWithSizeFromBuffer(buf, size, size, vips.InterestingNone, vips.SizeDown)
	if err != nil {
		return &UnsupportedMediaError{Backend: "govips", Reason: "failed to load logical image source", Err: err}
	}
	defer image.Close()
	return exportGovipsThumbnail(image, dst, format, quality)
}

func exportGovipsThumbnail(image *vips.ImageRef, dst io.Writer, format string, quality int) error {
	var data []byte
	var err error
	switch format {
	case "jpeg":
		params := vips.NewDefaultJPEGExportParams()
		params.Quality = quality
		params.StripMetadata = true
		data, _, err = image.Export(params)
	case "png":
		params := vips.NewDefaultPNGExportParams()
		params.StripMetadata = true
		data, _, err = image.Export(params)
	default:
		return fmt.Errorf("%w: thumbnail format %q", ErrUnsupportedMedia, format)
	}
	if err != nil {
		return &UnsupportedMediaError{Backend: "govips", Reason: "failed to export image thumbnail", Err: err}
	}
	_, err = dst.Write(data)
	return err
}

func ensureGovipsStarted() error {
	govipsStartupOnce.Do(func() {
		vips.LoggingSettings(func(string, vips.LogLevel, string) {}, vips.LogLevelError)
		govipsStartupErr = vips.Startup(nil)
	})
	return govipsStartupErr
}
