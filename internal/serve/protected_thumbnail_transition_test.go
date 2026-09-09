package serve

import (
	"bytes"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func TestProtectedCBZThumbnailUsesLogicalNameAndOpaqueStoragePath(t *testing.T) {
	page := tinyPNG(t, 8, 6, color.RGBA{B: 255, A: 255})
	archivePath := writeComic(t, map[string][]byte{"1.png": page})
	root := filepath.Dir(archivePath)
	logicalPath := filepath.Join(root, "library", "book.cbz")
	storagePath := filepath.Join(root, ".gooru-protected-v1", "aa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o700); err != nil {
		t.Fatal(err)
	}

	key := bytes.Repeat([]byte{0x52}, securekey.Size)
	encryptMediaFixture(t, archivePath, key)
	if err := os.Rename(archivePath, storagePath); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = key
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: root}}
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Media.ThumbnailSizes = []int{4}
	cfg.Media.ThumbnailFormat = "png"

	service := NewMediaService(cfg)
	recorder := httptest.NewRecorder()
	service.ServeDerivative(recorder, httptest.NewRequest(http.MethodGet, "/thumbnail?size=4", nil), types.FileInfo{
		Path:        logicalPath,
		StoragePath: storagePath,
		Hash:        "opaque-protected-comic",
	}, "thumbnail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("protected opaque comic thumbnail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Gooru-Cache"); got != "miss" {
		t.Fatalf("protected opaque comic thumbnail cache = %q, want miss", got)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("protected opaque comic thumbnail is empty")
	}
}
