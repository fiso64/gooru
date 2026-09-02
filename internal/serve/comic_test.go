package serve

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func tinyPNG(t *testing.T, width, height int, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, fill)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func writeComic(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.cbz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, body := range entries {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenComicArchiveUsesNaturalPageOrder(t *testing.T) {
	page := tinyPNG(t, 2, 2, color.White)
	path := writeComic(t, map[string][]byte{
		"pages/10.png": page,
		"pages/2.png":  page,
		"pages/1.png":  page,
		"notes.txt":    []byte("ignored"),
	})

	archive, err := openComicArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()

	got := make([]string, 0, len(archive.pages))
	for _, page := range archive.pages {
		got = append(got, filepath.Base(page.Name))
	}
	want := []string{"1.png", "2.png", "10.png"}
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pages = %v, want %v", got, want)
		}
	}
}

func TestServeComicManifestAndIndividualPage(t *testing.T) {
	first := tinyPNG(t, 3, 2, color.RGBA{R: 255, A: 255})
	second := tinyPNG(t, 2, 3, color.RGBA{B: 255, A: 255})
	path := writeComic(t, map[string][]byte{
		"02.png": second,
		"01.png": first,
	})
	service := NewMediaService(DefaultConfig(filepath.Join(t.TempDir(), "gooru.db")))
	file := types.FileInfo{Path: path, Hash: "comic-hash"}

	manifestRecorder := httptest.NewRecorder()
	manifestRequest := httptest.NewRequest(http.MethodGet, "/api/v1/comics/file-id", nil)
	service.ServeComic(manifestRecorder, manifestRequest, file, "file-id")
	if manifestRecorder.Code != http.StatusOK {
		t.Fatalf("manifest status = %d: %s", manifestRecorder.Code, manifestRecorder.Body.String())
	}
	var manifest comicManifest
	if err := json.Unmarshal(manifestRecorder.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Pages) != 2 || manifest.Pages[0].Name != "01.png" || manifest.Pages[1].Name != "02.png" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.Pages[1].URL != "/api/v1/comics/file-id/1" {
		t.Fatalf("second page URL = %q", manifest.Pages[1].URL)
	}

	pageRecorder := httptest.NewRecorder()
	pageRequest := httptest.NewRequest(http.MethodGet, "/api/v1/comics/file-id/1?page=1", nil)
	service.ServeComic(pageRecorder, pageRequest, file, "file-id")
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", pageRecorder.Code, pageRecorder.Body.String())
	}
	if got := pageRecorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if !bytes.Equal(pageRecorder.Body.Bytes(), second) {
		t.Fatal("served page bytes differ from selected archive entry")
	}
}

func TestCBZCoverThumbnailUsesFirstNaturalPage(t *testing.T) {
	first := tinyPNG(t, 40, 20, color.RGBA{R: 255, A: 255})
	second := tinyPNG(t, 20, 40, color.RGBA{B: 255, A: 255})
	path := writeComic(t, map[string][]byte{
		"10.png": second,
		"2.png":  first,
	})

	var encoded bytes.Buffer
	if err := thumbnailCBZFirstPage(path, &encoded, 10, "png"); err != nil {
		t.Fatal(err)
	}
	thumb, _, err := image.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := thumb.Bounds().Size(), (image.Point{X: 10, Y: 5}); got != want {
		t.Fatalf("thumbnail size = %v, want %v", got, want)
	}
}

func TestMediaServiceGeneratesCBZDerivative(t *testing.T) {
	path := writeComic(t, map[string][]byte{
		"1.png": tinyPNG(t, 16, 8, color.White),
	})
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = t.TempDir()
	cfg.Media.ThumbnailSizes = []int{8}
	cfg.Media.ThumbnailFormat = "png"
	service := NewMediaService(cfg)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/thumbnail?size=8", nil)
	service.ServeDerivative(recorder, request, types.FileInfo{Path: path, Hash: "cover-hash"}, "thumbnail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("thumbnail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	thumb, _, err := image.Decode(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := thumb.Bounds().Size(), (image.Point{X: 8, Y: 4}); got != want {
		t.Fatalf("thumbnail size = %v, want %v", got, want)
	}
}
