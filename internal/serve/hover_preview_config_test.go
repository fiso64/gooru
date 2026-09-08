package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHoverPlaybackConfigDefaultsAndOverrides(t *testing.T) {
	defaults := DefaultConfig(filepath.Join(t.TempDir(), "default.db"))
	if defaults.UI.HoverPlayVideos || !defaults.UI.HoverPlayGIFs {
		t.Fatalf("default hover playback flags = video:%v gif:%v, want video false and gif true", defaults.UI.HoverPlayVideos, defaults.UI.HoverPlayGIFs)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  hover_play_videos: false\n  hover_play_gifs: false\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.UI.HoverPlayVideos || cfg.UI.HoverPlayGIFs {
		t.Fatalf("hover playback flags = video:%v gif:%v, want both false", cfg.UI.HoverPlayVideos, cfg.UI.HoverPlayGIFs)
	}
}

func TestUIConfigExposesHoverPlaybackPreferences(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil)
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"hover_play_videos":true`) || !strings.Contains(rec.Body.String(), `"hover_play_gifs":true`) {
		t.Fatalf("response missing hover playback preferences: %s", rec.Body.String())
	}
}
