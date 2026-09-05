package serve

import (
	"bytes"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	handler := protectedAPICacheMiddleware(true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.ServeContent(w, r, types.FileInfo{Path: path, Size: int64(len(plaintext))})
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/files/file-id/content", nil)
	request.Header.Set("Range", "bytes=8-12")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got, want := recorder.Body.String(), string(plaintext[8:13]); got != want {
		t.Fatalf("range body = %q, want %q", got, want)
	}
	if got := recorder.Header().Get("Cache-Control"); got != protectedAPICacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, protectedAPICacheControl)
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

func TestProtectedImageThumbnailReadsEncryptedOriginal(t *testing.T) {
	plaintext := tinyPNG(t, 32, 24, color.RGBA{R: 200, G: 40, B: 90, A: 255})
	path := writeNamedMediaFile(t, "secret.png", plaintext)
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x35}, securekey.Size)
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "png"
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	service := NewMediaService(cfg)
	recorder := httptest.NewRecorder()
	service.ServeDerivative(recorder, httptest.NewRequest(http.MethodGet, "/thumbnail?size=16", nil), types.FileInfo{Path: path, Hash: "protected-image"}, "thumbnail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("protected image thumbnail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	decoded, _, err := image.Decode(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode protected image thumbnail: %v", err)
	}
	if got := decoded.Bounds().Dx(); got != 16 {
		t.Fatalf("thumbnail width = %d, want 16", got)
	}
	if got := recorder.Header().Get("X-Gooru-Cache"); got != "bypass" {
		t.Fatalf("protected image thumbnail cache = %q, want bypass", got)
	}
	if _, err := os.Stat(cfg.Media.CacheDir); !os.IsNotExist(err) {
		t.Fatalf("protected image thumbnail must not create plaintext cache, stat error = %v", err)
	}
}

func TestProtectedVideoThumbnailStreamsDecryptedSource(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	path := writeTestVideo(t, ffmpeg, 32, 24)
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x61}, securekey.Size)
	cfg.Tools.FFmpegPath = ffmpeg
	cfg.Tools.FFprobePath = ffprobe
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "png"
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	service := NewMediaService(cfg)
	recorder := httptest.NewRecorder()
	service.ServeDerivative(recorder, httptest.NewRequest(http.MethodGet, "/thumbnail?size=16", nil), types.FileInfo{Path: path, Hash: "protected-video"}, "thumbnail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("protected video thumbnail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, _, err := image.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
		t.Fatalf("decode protected video thumbnail: %v", err)
	}
	if got := recorder.Header().Get("X-Gooru-Cache"); got != "bypass" {
		t.Fatalf("protected video thumbnail cache = %q, want bypass", got)
	}
	if _, err := os.Stat(cfg.Media.CacheDir); !os.IsNotExist(err) {
		t.Fatalf("protected video thumbnail must not create plaintext cache, stat error = %v", err)
	}
}
