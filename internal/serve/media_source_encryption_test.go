package serve

import (
	"bytes"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func encryptMediaFixture(t *testing.T, path string, key []byte) {
	t.Helper()
	plaintext, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(out, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedContentReadsEncryptedOriginalWithRange(t *testing.T) {
	plaintext := []byte("private-media-0123456789")
	path := writeNamedMediaFile(t, "secret.bin", plaintext)
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x42}, securekey.Size)
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	ciphertext, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("encrypted original contains plaintext media bytes")
	}

	service := NewMediaService(cfg)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/content", nil)
	request.Header.Set("Range", "bytes=8-12")
	service.ServeContent(recorder, request, types.FileInfo{Path: path, Size: int64(len(plaintext))})

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got, want := recorder.Body.String(), string(plaintext[8:13]); got != want {
		t.Fatalf("range body = %q, want %q", got, want)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
}

func TestProtectedComicReadsEncryptedArchiveWithoutPlaintextMaterialization(t *testing.T) {
	page := tinyPNG(t, 6, 4, color.RGBA{G: 255, A: 255})
	path := writeComic(t, map[string][]byte{"1.png": page})
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x24}, securekey.Size)
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Media.ThumbnailSizes = []int{4}
	cfg.Media.ThumbnailFormat = "png"
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	service := NewMediaService(cfg)
	pageRecorder := httptest.NewRecorder()
	pageRequest := httptest.NewRequest(http.MethodGet, "/api/v1/comics/file-id/0?page=0", nil)
	service.ServeComic(pageRecorder, pageRequest, types.FileInfo{Path: path}, "file-id")
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("comic page status = %d: %s", pageRecorder.Code, pageRecorder.Body.String())
	}
	if !bytes.Equal(pageRecorder.Body.Bytes(), page) {
		t.Fatal("decrypted comic page differs from archived plaintext")
	}

	thumbRecorder := httptest.NewRecorder()
	thumbRequest := httptest.NewRequest(http.MethodGet, "/thumbnail?size=4", nil)
	service.ServeDerivative(thumbRecorder, thumbRequest, types.FileInfo{Path: path, Hash: "encrypted-comic"}, "thumbnail")
	if thumbRecorder.Code != http.StatusOK {
		t.Fatalf("comic thumbnail status = %d: %s", thumbRecorder.Code, thumbRecorder.Body.String())
	}
	if got := thumbRecorder.Header().Get("X-Gooru-Cache"); got != "bypass" {
		t.Fatalf("protected comic thumbnail cache = %q, want bypass", got)
	}
	if _, err := os.Stat(cfg.Media.CacheDir); !os.IsNotExist(err) {
		t.Fatalf("protected comic thumbnail must not create plaintext cache, stat error = %v", err)
	}
}
