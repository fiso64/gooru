package serve

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"gooru.local/types"
)

type previewQualityThumbnailer struct {
	formats []string
}

func (t *previewQualityThumbnailer) BackendVersion() string { return "preview-quality-test-v1" }

func (t *previewQualityThumbnailer) Thumbnail(_ string, dst io.Writer, size int, format string) error {
	t.formats = append(t.formats, format)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*17 + y*11) % 256),
				G: uint8((x*7 + y*23) % 256),
				B: uint8((x*29 + y*3) % 256),
				A: 255,
			})
		}
	}
	switch format {
	case "png":
		return png.Encode(dst, img)
	case "jpeg":
		return jpeg.Encode(dst, img, &jpeg.Options{Quality: derivativeJPEGQuality})
	default:
		return ErrUnsupportedMedia
	}
}

func TestConfiguredPreviewJPEGQualityIsAppliedAfterLosslessGeneration(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.PreviewJPEGQuality = 25
	media := NewMediaService(cfg)
	thumbnailer := &previewQualityThumbnailer{}
	media.thumbnailer = thumbnailer

	var low bytes.Buffer
	if err := media.generateDerivative(types.FileInfo{Path: "sample.jpg"}, &low, 96, "jpeg", "preview"); err != nil {
		t.Fatalf("generate low-quality preview: %v", err)
	}
	if !reflect.DeepEqual(thumbnailer.formats, []string{"png"}) {
		t.Fatalf("preview backend formats = %v, want lossless png intermediate", thumbnailer.formats)
	}
	if _, err := jpeg.Decode(bytes.NewReader(low.Bytes())); err != nil {
		t.Fatalf("configured preview is not a JPEG: %v", err)
	}

	thumbnailer.formats = nil
	media.cfg.Media.PreviewJPEGQuality = 90
	var high bytes.Buffer
	if err := media.generateDerivative(types.FileInfo{Path: "sample.jpg"}, &high, 96, "jpeg", "preview"); err != nil {
		t.Fatalf("generate high-quality preview: %v", err)
	}
	if high.Len() <= low.Len() {
		t.Fatalf("high-quality preview size = %d, low-quality = %d; expected quality setting to affect encoding", high.Len(), low.Len())
	}
}

func TestPreviewJPEGQualityOnlyChangesPreviewCacheKey(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	media := NewMediaService(cfg)
	media.thumbnailer = &previewQualityThumbnailer{}
	file := types.FileInfo{Path: "sample.jpg", Hash: "hash"}

	media.cfg.Media.PreviewJPEGQuality = 40
	previewLow := media.derivativeRelativePath(file, "preview", 1280, "jpeg")
	thumbnailLow := media.derivativeRelativePath(file, "thumbnail", 256, "jpeg")
	media.cfg.Media.PreviewJPEGQuality = 90
	previewHigh := media.derivativeRelativePath(file, "preview", 1280, "jpeg")
	thumbnailHigh := media.derivativeRelativePath(file, "thumbnail", 256, "jpeg")

	if previewLow == previewHigh {
		t.Fatal("preview cache key did not change with JPEG quality")
	}
	if thumbnailLow != thumbnailHigh {
		t.Fatal("grid thumbnail cache key changed with preview JPEG quality")
	}
}
