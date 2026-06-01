package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDefaultsAreValid(t *testing.T) {
	cfg, err := LoadConfig("", filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig default failed: %v", err)
	}
	if cfg.Server.Listen != DefaultListenAddress {
		t.Fatalf("unexpected listen address %q", cfg.Server.Listen)
	}
	if cfg.Uploads.Enabled {
		t.Fatal("uploads should default disabled until a directory is configured")
	}
	if cfg.Server.ExposePaths {
		t.Fatal("server.expose_paths should default false")
	}
	if cfg.Server.FrontendDir == "" {
		t.Fatal("server.frontend_dir should point at the static frontend build by default")
	}
	if cfg.Jobs.CompletedTTL == 0 {
		t.Fatal("completed TTL was not parsed")
	}
	if cfg.Jobs.MaxQueued != 100 || cfg.Jobs.MaxRunning != 2 || cfg.Jobs.MaxResultBytes != 10<<20 {
		t.Fatalf("unexpected job defaults: %+v", cfg.Jobs)
	}
}

func TestLoadConfigRejectsTokenFromEnvironment(t *testing.T) {
	t.Setenv("GOORU_TEST_TOKEN", " secret-token ")
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "127.0.0.1:5678"
database:
  path: "/tmp/gooru.db"
auth:
  token_env: "GOORU_TEST_TOKEN"
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "30m"
  max_queued: 5
  max_running: 3
  max_result_bytes: 1024
tools:
  ffmpeg_path: "ffmpeg"
  ffprobe_path: "ffprobe"
logging:
  level: "info"
`)
	_, err := LoadConfig(path, "", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected token deprecation error, got %v", err)
	}
}

func TestLoadConfigRejectsAuthTokenOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "0.0.0.0:5678"
database:
  path: "/tmp/gooru.db"
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "30m"
`)
	_, err := LoadConfig(path, "", Overrides{AuthToken: " cli-token "})
	if err == nil || !strings.Contains(err.Error(), "--auth-token is no longer supported") {
		t.Fatalf("expected token override deprecation error, got %v", err)
	}
}

func TestLoadConfigRejectsConflictingTokenSources(t *testing.T) {
	t.Setenv("GOORU_TEST_TOKEN", "env-token")
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "127.0.0.1:5678"
database:
  path: "/tmp/gooru.db"
auth:
  token: "direct-token"
  token_env: "GOORU_TEST_TOKEN"
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "30m"
`)
	_, err := LoadConfig(path, "", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected token deprecation error, got %v", err)
	}
}

func TestLoadConfigRejectsBlankDirectToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "127.0.0.1:5678"
database:
  path: "/tmp/gooru.db"
auth:
  token: "   "
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "30m"
`)
	_, err := LoadConfig(path, "", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected token deprecation error, got %v", err)
	}
}

func TestLoadConfigRejectsBlankTokenOverride(t *testing.T) {
	_, err := LoadConfig("", filepath.Join(t.TempDir(), "gooru.db"), Overrides{AuthToken: "  "})
	if err == nil || !strings.Contains(err.Error(), "--auth-token is no longer supported") {
		t.Fatalf("expected token override deprecation error, got %v", err)
	}
}

func TestLoadConfigRejectsNonLoopbackWithAuthDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "0.0.0.0:5678"
database:
  path: "/tmp/gooru.db"
auth:
  enabled: false
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "1h"
`)
	_, err := LoadConfig(path, "", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "refusing auth.enabled=false") {
		t.Fatalf("expected unsafe bind error, got %v", err)
	}
}

func TestLoadConfigRejectsEnabledUploadsWithoutDirectory(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Enabled = true
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "uploads.enabled requires") {
		t.Fatalf("expected upload directory validation error, got %v", err)
	}
}

func TestLoadConfigRejectsUnsupportedThumbnailFormat(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.ThumbnailFormat = "webp"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "media.thumbnail_format must be one of: jpeg, png") {
		t.Fatalf("expected thumbnail format validation error, got %v", err)
	}
}

func TestLoadConfigRejectsInvalidJobLimits(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Jobs.MaxQueued = 0
	cfg.Jobs.MaxRunning = 0
	cfg.Jobs.MaxResultBytes = 0

	err := cfg.Validate()
	if err == nil ||
		!strings.Contains(err.Error(), "jobs.max_queued") ||
		!strings.Contains(err.Error(), "jobs.max_running") ||
		!strings.Contains(err.Error(), "jobs.max_result_bytes") {
		t.Fatalf("expected job limit validation errors, got %v", err)
	}
}

func TestDefaultYAMLParses(t *testing.T) {
	data, err := DefaultYAML(filepath.Join(t.TempDir(), "gooru.db"))
	if err != nil {
		t.Fatalf("DefaultYAML failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "default.yaml")
	writeConfig(t, path, string(data))
	if _, err := LoadConfig(path, "", Overrides{}); err != nil {
		t.Fatalf("generated default YAML should parse and validate: %v\n%s", err, data)
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "127.0.0.1:5678"
  typo_cors_origin: ["https://example.test"]
database:
  path: "/tmp/gooru.db"
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "1h"
`)
	_, err := LoadConfig(path, "", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "field typo_cors_origin not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func writeConfig(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
