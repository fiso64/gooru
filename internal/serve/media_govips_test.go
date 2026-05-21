//go:build govips

package serve

import (
	"bytes"
	"image"
	_ "image/jpeg"
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
	if out.Len() == 0 {
		t.Fatal("empty govips thumbnail")
	}
	img, _, err := image.Decode(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("decode govips thumbnail: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() > 16 || bounds.Dy() > 16 {
		t.Fatalf("expected bounded govips thumbnail, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}
