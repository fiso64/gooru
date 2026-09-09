package serve

import (
	"bytes"
	"image"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func TestProtectedVideoThumbnailContainerParity(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}

	tests := []struct {
		name  string
		ext   string
		codec string
	}{
		{name: "webm", ext: ".webm", codec: "libvpx-vp9"},
		{name: "matroska", ext: ".mkv", codec: "mpeg4"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "video"+tt.ext)
			cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=32x24:d=1", "-t", "1", "-c:v", tt.codec, path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("generate %s fixture: %v: %s", tt.name, err, output)
			}

			cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
			cfg.Encryption.Enabled = true
			cfg.Encryption.Key = bytes.Repeat([]byte{byte(0x70 + i)}, securekey.Size)
			cfg.Tools.FFmpegPath = ffmpeg
			cfg.Tools.FFprobePath = ffprobe
			cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
			cfg.Media.ThumbnailSizes = []int{16}
			cfg.Media.ThumbnailFormat = "png"
			encryptMediaFixture(t, path, cfg.Encryption.Key)

			service := NewMediaService(cfg)
			recorder := httptest.NewRecorder()
			service.ServeDerivative(recorder, httptest.NewRequest(http.MethodGet, "/thumbnail?size=16", nil), types.FileInfo{Path: path, Hash: "protected-" + tt.name}, "thumbnail")
			if recorder.Code != http.StatusOK {
				t.Fatalf("protected %s thumbnail status = %d: %s", tt.name, recorder.Code, recorder.Body.String())
			}
			if _, _, err := image.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
				t.Fatalf("decode protected %s thumbnail: %v", tt.name, err)
			}
		})
	}
}
