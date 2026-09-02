package serve

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestOpenComicArchiveReaderDoesNotRequireFilesystemPath(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	page, err := zw.Create("001.png")
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(page, img); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	reader := bytes.NewReader(archive.Bytes())
	comic, err := openComicArchiveReader("memory.cbz", reader, int64(reader.Len()), nil)
	if err != nil {
		t.Fatalf("open random-access CBZ: %v", err)
	}
	defer comic.Close()
	if len(comic.pages) != 1 || comic.pages[0].Name != "001.png" {
		t.Fatalf("unexpected pages: %#v", comic.pages)
	}
}
