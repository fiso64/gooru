package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfigWithUploadTarget(t *testing.T, targetPath string) Config {
	t.Helper()
	dir := t.TempDir()
	cfg := DefaultConfig(filepath.Join(dir, "state", "gooru.db"))
	cfg.Server.FrontendDir = filepath.Join(dir, "frontend")
	cfg.Media.CacheDir = filepath.Join(dir, "cache")
	cfg.Tools.FFmpegPath = "ffmpeg"
	cfg.Tools.FFprobePath = "ffprobe"
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: targetPath}}
	return cfg
}

func TestValidateRejectsUploadTargetContainingProtectedApplicationPaths(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gooru")
	cases := []struct {
		name      string
		configure func(*Config)
		want      string
	}{
		{name: "database", configure: func(cfg *Config) { cfg.Database.Path = filepath.Join(target, "state", "gooru.db") }, want: "database.path"},
		{name: "encryption key", configure: func(cfg *Config) { cfg.Encryption.KeyFile = filepath.Join(target, "secrets", "encryption.key") }, want: "encryption.key_file"},
		{name: "media cache", configure: func(cfg *Config) { cfg.Media.CacheDir = filepath.Join(target, "cache") }, want: "media.cache_dir"},
		{name: "frontend", configure: func(cfg *Config) { cfg.Server.FrontendDir = filepath.Join(target, "frontend") }, want: "server.frontend_dir"},
		{name: "ffmpeg", configure: func(cfg *Config) { cfg.Tools.FFmpegPath = filepath.Join(target, "bin", "ffmpeg") }, want: "tools.ffmpeg_path"},
		{name: "ffprobe", configure: func(cfg *Config) { cfg.Tools.FFprobePath = filepath.Join(target, "bin", "ffprobe") }, want: "tools.ffprobe_path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfigWithUploadTarget(t, target)
			tc.configure(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected overlap with %s to fail, got %v", tc.want, err)
			}
		})
	}
}

func TestValidateRejectsProtectedDirectoryContainingUploadTarget(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	target := filepath.Join(cache, "uploads")
	cfg := validConfigWithUploadTarget(t, target)
	cfg.Media.CacheDir = cache
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "media.cache_dir") {
		t.Fatalf("expected cache/upload containment to fail, got %v", err)
	}
}

func TestValidateRejectsUploadTargetOverlapThroughExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	realRoot := filepath.Join(dir, "real")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(dir, "linked")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	target := filepath.Join(linkedRoot, "uploads")
	cfg := validConfigWithUploadTarget(t, target)
	cfg.Database.Path = filepath.Join(realRoot, "uploads", "state", "gooru.db")
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "database.path") {
		t.Fatalf("expected resolved symlink overlap to fail, got %v", err)
	}
}

func TestValidateRejectsFuturePathOverlapBelowSymlinkedParent(t *testing.T) {
	dir := t.TempDir()
	realRoot := filepath.Join(dir, "real")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(dir, "linked")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	target := filepath.Join(linkedRoot, "future", "uploads")
	cfg := validConfigWithUploadTarget(t, target)
	cfg.Server.FrontendDir = filepath.Join(realRoot, "future")
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "server.frontend_dir") {
		t.Fatalf("expected future path overlap below resolved parent to fail, got %v", err)
	}
}

func TestValidateAllowsSeparatedApplicationAndUploadPaths(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfigWithUploadTarget(t, filepath.Join(dir, "uploads"))
	cfg.Database.Path = filepath.Join(dir, "state", "gooru.db")
	cfg.Encryption.KeyFile = filepath.Join(dir, "secrets", "encryption.key")
	cfg.Media.CacheDir = filepath.Join(dir, "cache")
	cfg.Server.FrontendDir = filepath.Join(dir, "frontend")
	cfg.Tools.FFmpegPath = filepath.Join(dir, "bin", "ffmpeg")
	cfg.Tools.FFprobePath = filepath.Join(dir, "bin", "ffprobe")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("separated paths should be valid: %v", err)
	}
}
