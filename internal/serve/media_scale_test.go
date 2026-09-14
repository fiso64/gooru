package serve

import (
	"image"
	"testing"
)

func TestScaleImageUsesShortSide(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
		size   int
		wantW  int
		wantH  int
	}{
		{name: "portrait", width: 1200, height: 1600, size: 256, wantW: 256, wantH: 341},
		{name: "landscape", width: 1600, height: 1200, size: 256, wantW: 341, wantH: 256},
		{name: "square", width: 1200, height: 1200, size: 256, wantW: 256, wantH: 256},
		{name: "does not upscale", width: 200, height: 400, size: 256, wantW: 200, wantH: 400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := image.NewRGBA(image.Rect(0, 0, tc.width, tc.height))
			got := scaleImage(src, tc.size).Bounds()
			if got.Dx() != tc.wantW || got.Dy() != tc.wantH {
				t.Fatalf("scaleImage(%dx%d, %d) = %dx%d, want %dx%d", tc.width, tc.height, tc.size, got.Dx(), got.Dy(), tc.wantW, tc.wantH)
			}
		})
	}
}
