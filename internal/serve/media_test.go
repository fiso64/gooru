package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gooru.local/types"
)

func TestContentRouteSupportsRanges(t *testing.T) {
	file := writeMediaFile(t, []byte("0123456789"))
	server := newMediaTestServer(t, types.FileInfo{ID: 1, Path: file, Hash: "hash-content", Size: 10})
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(1)+"/content")
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "2345" {
		t.Fatalf("unexpected range body %q", got)
	}
}

func TestMediaRoutesRequireSession(t *testing.T) {
	file := writeMediaFile(t, []byte("0123456789"))
	server := newMediaAuthTestServer(t, types.FileInfo{ID: 2, Path: file, Hash: "hash-content", Size: 10})
	attachTestAuth(t, server)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(2)+"/content", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON error response, got %q", got)
	}
}

func TestMediaContentAcceptsAuthCookieForBrowserOpen(t *testing.T) {
	file := writeMediaFile(t, []byte("0123456789"))
	server := newMediaAuthTestServer(t, types.FileInfo{ID: 3, Path: file, Hash: "hash-cookie", Size: 10})
	auth := attachTestAuth(t, server)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(3)+"/content", nil)
	addAuthCookie(req, server.cfg, auth)
	req.Header.Set("Range", "bytes=4-6")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "456" {
		t.Fatalf("unexpected range body %q", got)
	}
}

func TestThumbnailRouteGeneratesAndCaches(t *testing.T) {
	imagePath := writePNGImage(t)
	server := newMediaTestServer(t, types.FileInfo{ID: 7, Path: imagePath, Hash: "hash-image", Size: 100})

	for i, wantCache := range []string{"miss", "hit"} {
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(7)+"/thumbnail?size=16")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Gooru-Cache"); got != wantCache {
			t.Fatalf("request %d expected cache %q, got %q", i+1, wantCache, got)
		}
		if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
			t.Fatalf("expected image/jpeg, got %q", got)
		}
		if rec.Body.Len() == 0 {
			t.Fatal("empty thumbnail response")
		}
	}
}

func TestThumbnailRouteSerializesConcurrentCacheMisses(t *testing.T) {
	imagePath := writePNGImage(t)
	server := newMediaTestServer(t, types.FileInfo{ID: 8, Path: imagePath, Hash: "hash-concurrent", Size: 100})
	thumbnailer := &blockingThumbnailer{
		delegate: GoImageThumbnailer{},
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	server.media.thumbnailer = thumbnailer

	var wg sync.WaitGroup
	codes := make(chan int, 2)
	request := func() {
		defer wg.Done()
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(8)+"/thumbnail?size=16")
		server.Handler().ServeHTTP(rec, req)
		codes <- rec.Code
	}

	wg.Add(1)
	go request()
	select {
	case <-thumbnailer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first thumbnail request did not start generation")
	}
	wg.Add(1)
	go request()
	close(thumbnailer.release)
	wg.Wait()
	close(codes)

	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("expected concurrent thumbnail request to succeed, got %d", code)
		}
	}
	if got := thumbnailer.calls.Load(); got != 1 {
		t.Fatalf("expected one thumbnail generation, got %d", got)
	}
	server.media.cacheMu.Lock()
	locks := len(server.media.cacheLocks)
	server.media.cacheMu.Unlock()
	if locks != 0 {
		t.Fatalf("expected cache lock cleanup, got %d locks", locks)
	}
}

func TestImageThumbnailerFallsBackWhenVipsMissing(t *testing.T) {
	imagePath := writePNGImage(t)
	thumbnailer := &MediaThumbnailer{
		imagePrimary:  unavailableThumbnailer{},
		imageFallback: GoImageThumbnailer{},
		version:       "test",
	}
	var out bytes.Buffer

	if err := thumbnailer.Thumbnail(imagePath, &out, 16, "jpeg"); err != nil {
		t.Fatalf("expected pure-Go image fallback to generate thumbnail: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("empty fallback thumbnail")
	}
}

func TestFFmpegThumbnailArgsSeekBeforeInput(t *testing.T) {
	offset := 1500 * time.Millisecond
	args := ffmpegThumbnailArgs("in.mp4", "out.jpg", 128, "jpeg", &offset)
	got := strings.Join(args, " ")
	if !strings.Contains(got, "-ss 1.500 -i in.mp4") {
		t.Fatalf("expected seek before input, got %v", args)
	}
}

func TestVideoThumbnailOffsetStrategy(t *testing.T) {
	cases := []struct {
		name     string
		duration string
		want     time.Duration
	}{
		{name: "ten percent", duration: "20", want: 2 * time.Second},
		{name: "floor", duration: "1", want: 500 * time.Millisecond},
		{name: "cap", duration: "120", want: 3 * time.Second},
		{name: "short duration max", duration: "0.6", want: 350 * time.Millisecond},
		{name: "bad duration", duration: "not-a-number", want: 3 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ffprobe := writeFakeFFprobe(t, tc.duration, 0)
			got, ok := videoThumbnailOffset(context.Background(), ffprobe, "video.mp4")
			if !ok {
				t.Fatal("expected offset strategy")
			}
			if got != tc.want {
				t.Fatalf("expected offset %s, got %s", tc.want, got)
			}
		})
	}
}

func TestVideoThumbnailOffsetFallsBackWhenProbeFails(t *testing.T) {
	ffprobe := writeFakeFFprobe(t, "", 2)
	got, ok := videoThumbnailOffset(context.Background(), ffprobe, "video.mp4")
	if !ok {
		t.Fatal("expected fallback offset strategy")
	}
	if got != 3*time.Second {
		t.Fatalf("expected 3s fallback offset, got %s", got)
	}
}

func TestFFmpegThumbnailFallsBackToFirstFrameWhenOffsetFails(t *testing.T) {
	ffmpeg, logPath := writeFallbackFFmpeg(t)
	ffprobe := writeFakeFFprobe(t, "20", 0)
	thumbnailer := NewFFmpegVideoThumbnailer(ffmpeg, ffprobe)
	var out bytes.Buffer

	if err := thumbnailer.Thumbnail("video.mp4", &out, 16, "jpeg"); err != nil {
		t.Fatalf("generate thumbnail: %v", err)
	}
	if got := out.String(); got != "fallback derivative\n" {
		t.Fatalf("unexpected derivative output %q", got)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read ffmpeg log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected offset and fallback ffmpeg attempts, got %d: %q", len(lines), logData)
	}
	if !strings.Contains(lines[0], "-ss 2.000 -i video.mp4") {
		t.Fatalf("expected first attempt to use probed offset before input, got %q", lines[0])
	}
	if strings.Contains(lines[1], "-ss") {
		t.Fatalf("expected fallback attempt to omit seek offset, got %q", lines[1])
	}
}

func TestVideoThumbnailRouteUsesFFmpegBackend(t *testing.T) {
	videoPath := writeNamedMediaFile(t, "video.mp4", []byte("fake video"))
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "media-cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "jpeg"
	cfg.Media.PreviewSize = 32
	ffmpeg := writeFakeFFmpeg(t)
	cfg.Tools.FFmpegPath = ffmpeg
	cfg.Tools.FFprobePath = ffmpeg
	server := NewServerWithLibrary(cfg, mediaLibrary{file: types.FileInfo{ID: 9, Path: videoPath, Hash: "hash-video", Size: 10}})
	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(9)+"/thumbnail?size=16")

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected video thumbnail 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", got)
	}
	if got := rec.Header().Get("X-Gooru-Cache"); got != "miss" {
		t.Fatalf("expected cache miss, got %q", got)
	}
	if rec.Body.String() != "fake derivative\n" {
		t.Fatalf("unexpected fake derivative body %q", rec.Body.String())
	}
}

func TestFFmpegVideoThumbnailerConstrainsMaxDimension(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		ffprobe = filepath.Join(t.TempDir(), "missing-ffprobe")
	}
	thumbnailer := NewFFmpegVideoThumbnailer(ffmpeg, ffprobe)
	cases := []struct {
		name   string
		width  int
		height int
		wantW  int
		wantH  int
	}{
		{name: "square", width: 64, height: 64, wantW: 16, wantH: 16},
		{name: "landscape", width: 64, height: 32, wantW: 16, wantH: 8},
		{name: "portrait", width: 32, height: 64, wantW: 8, wantH: 16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			videoPath := writeTestVideo(t, ffmpeg, tc.width, tc.height)
			var out bytes.Buffer
			if err := thumbnailer.Thumbnail(videoPath, &out, 16, "jpeg"); err != nil {
				t.Fatalf("generate thumbnail: %v", err)
			}
			img, _, err := image.Decode(bytes.NewReader(out.Bytes()))
			if err != nil {
				t.Fatalf("decode generated thumbnail: %v", err)
			}
			bounds := img.Bounds()
			if gotW, gotH := bounds.Dx(), bounds.Dy(); gotW != tc.wantW || gotH != tc.wantH {
				t.Fatalf("expected %dx%d thumbnail, got %dx%d", tc.wantW, tc.wantH, gotW, gotH)
			}
		})
	}
}

func TestVideoThumbnailRouteReportsMissingFFmpeg(t *testing.T) {
	videoPath := writeNamedMediaFile(t, "video.mp4", []byte("fake video"))
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "media-cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "jpeg"
	cfg.Media.PreviewSize = 32
	cfg.Tools.FFmpegPath = filepath.Join(t.TempDir(), "missing-ffmpeg")
	cfg.Tools.FFprobePath = filepath.Join(t.TempDir(), "missing-ffprobe")
	server := NewServerWithLibrary(cfg, mediaLibrary{file: types.FileInfo{ID: 10, Path: videoPath, Hash: "hash-video-missing", Size: 10}})
	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(10)+"/thumbnail?size=16")

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	details, ok := payload.Error.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("expected machine-readable details, got %#v", payload.Error.Details)
	}
	if details["backend"] != "ffmpeg" || details["reason"] == "" {
		t.Fatalf("unexpected unsupported details: %#v", details)
	}
}

func newMediaTestServer(t *testing.T, file types.FileInfo) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "media-cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "jpeg"
	cfg.Media.PreviewSize = 32
	return NewServerWithLibrary(cfg, mediaLibrary{file: file})
}

func newMediaAuthTestServer(t *testing.T, file types.FileInfo) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "media-cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "jpeg"
	cfg.Media.PreviewSize = 32
	return NewServerWithLibrary(cfg, mediaLibrary{file: file})
}

func writeMediaFile(t *testing.T, data []byte) string {
	t.Helper()
	return writeNamedMediaFile(t, "content.bin", data)
}

func writeNamedMediaFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	return path
}

func writeFakeFFmpeg(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
if [ "$1" = "-version" ]; then
  echo "fake ffmpeg 1.0"
  exit 0
fi
last=""
for arg in "$@"; do
  last="$arg"
done
printf "fake derivative\n" > "$last"
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

func writeFallbackFFmpeg(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	logPath := filepath.Join(dir, "ffmpeg.log")
	script := `#!/bin/sh
if [ "$1" = "-version" ]; then
  echo "fake ffmpeg 1.0"
  exit 0
fi
last=""
args=""
for arg in "$@"; do
  last="$arg"
  args="$args $arg"
done
printf "%s\n" "$args" >> "` + logPath + `"
if echo " $args " | grep -q " -ss "; then
  : > "$last"
  exit 0
fi
printf "fallback derivative\n" > "$last"
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write fallback ffmpeg: %v", err)
	}
	return path, logPath
}

func writeFakeFFprobe(t *testing.T, output string, exitCode int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffprobe")
	script := `#!/bin/sh
if [ "$1" = "-version" ]; then
  echo "fake ffprobe 1.0"
  exit 0
fi
if [ "` + strconv.Itoa(exitCode) + `" != "0" ]; then
  exit ` + strconv.Itoa(exitCode) + `
fi
printf "%s\n" "` + output + `"
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write fake ffprobe: %v", err)
	}
	return path
}

func writeTestVideo(t *testing.T, ffmpeg string, width int, height int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "video.mp4")
	input := "color=c=red:s=" + strconv.Itoa(width) + "x" + strconv.Itoa(height) + ":d=0.1"
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", input, "-frames:v", "1", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create test video: %v: %s", err, output)
	}
	return path
}

type failingThumbnailer struct{}

func (failingThumbnailer) Thumbnail(string, io.Writer, int, string) error {
	return errors.New("fallback should not run")
}

func (failingThumbnailer) BackendVersion() string {
	return "failing"
}

type unavailableThumbnailer struct{}

func (unavailableThumbnailer) Thumbnail(string, io.Writer, int, string) error {
	return &UnsupportedMediaError{Backend: "test", Reason: "unavailable", Err: ErrUnsupportedMedia}
}

func (unavailableThumbnailer) BackendVersion() string {
	return "unavailable"
}

func writePNGImage(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 9), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatalf("write png: %v", err)
	}
	return path
}

type mediaLibrary struct {
	file types.FileInfo
}

func (l mediaLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return []types.FileInfo{l.file}, nil
}

func (l mediaLibrary) GetFile(_ context.Context, id int64) (types.FileInfo, error) {
	if id != l.file.ID {
		return types.FileInfo{}, ErrNotFound
	}
	return l.file, nil
}

func (mediaLibrary) ListTags(_ context.Context, _ bool) ([]TagDTO, error) {
	return nil, nil
}

func (l mediaLibrary) PublicFileID(file types.FileInfo) string {
	return fallbackPublicFileID(file.ID)
}

func (l mediaLibrary) ResolveFileID(_ context.Context, id string) (int64, error) {
	return fallbackResolveFileID(id)
}

type blockingThumbnailer struct {
	delegate Thumbnailer
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
	calls    atomic.Int32
}

func (t *blockingThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	t.calls.Add(1)
	t.once.Do(func() {
		close(t.started)
		<-t.release
	})
	return t.delegate.Thumbnail(src, dst, size, format)
}

func (t *blockingThumbnailer) BackendVersion() string {
	return "blocking-" + t.delegate.BackendVersion()
}
