//go:build govips

package serve

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"os"
	"testing"
)

func TestImageThumbnailerUsesGovipsPrimary(t *testing.T) {
	if err := ensureGovipsStarted(); err != nil {
		t.Skipf("govips is unavailable: %v", err)
	}
	imagePath := writePNGImage(t)
	thumbnailer := &MediaThumbnailer{
		imagePrimary:  GovipsImageThumbnailer{},
		imageFallback: failingThumbnailer{},
		version:       "test",
	}
	var out bytes.Buffer

	if err := thumbnailer.Thumbnail(imagePath, &out, 16, "jpeg"); err != nil {
		t.Fatalf("expected govips backend to generate thumbnail: %v", err)
	}
	assertBoundedGovipsThumbnail(t, out.Bytes(), 16)
}

func TestGovipsThumbnailerReadsLogicalSourceBuffer(t *testing.T) {
	if err := ensureGovipsStarted(); err != nil {
		t.Skipf("govips is unavailable: %v", err)
	}
	imagePath := writePNGImage(t)
	input, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := thumbnailImageSourcePrimary(imagePath, bytes.NewReader(input), &out, 16, "jpeg", derivativeJPEGQuality); err != nil {
		t.Fatalf("expected govips to generate thumbnail from logical source buffer: %v", err)
	}
	assertBoundedGovipsThumbnail(t, out.Bytes(), 16)
}

func assertBoundedGovipsThumbnail(t *testing.T, data []byte, size int) {
	t.Helper()
	if len(data) == 0 {
		t.Fatal("empty govips thumbnail")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode govips thumbnail: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() > size || bounds.Dy() > size {
		t.Fatalf("expected bounded govips thumbnail, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}
