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
	if cfg.Jobs.CompletedTTL == 0 {
		t.Fatal("completed TTL was not parsed")
	}
}

func TestLoadConfigResolvesTokenFromEnvironment(t *testing.T) {
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
tools:
  ffmpeg_path: "ffmpeg"
  ffprobe_path: "ffprobe"
logging:
  level: "info"
`)
	cfg, err := LoadConfig(path, "", Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Auth.Token != "secret-token" {
		t.Fatalf("token was not trimmed/resolved: %q", cfg.Auth.Token)
	}
}

func TestLoadConfigAuthTokenOverrideHasFinalPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
server:
  listen: "0.0.0.0:5678"
database:
  path: "/tmp/gooru.db"
auth:
  token_env: "GOORU_MISSING_TOKEN"
media:
  thumbnail_sizes: [256]
  thumbnail_format: "jpeg"
  preview_size: 1280
jobs:
  completed_ttl: "30m"
`)
	cfg, err := LoadConfig(path, "", Overrides{AuthToken: " cli-token "})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Auth.Token != "cli-token" {
		t.Fatalf("token override was not trimmed/applied last: %q", cfg.Auth.Token)
	}
	if cfg.Auth.TokenEnv != "" {
		t.Fatalf("token_env should be cleared by explicit override, got %q", cfg.Auth.TokenEnv)
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
	if err == nil || !strings.Contains(err.Error(), "configure only one auth token source") {
		t.Fatalf("expected token source conflict error, got %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "auth.token must not be blank") {
		t.Fatalf("expected blank token error, got %v", err)
	}
}

func TestLoadConfigRejectsBlankTokenOverride(t *testing.T) {
	_, err := LoadConfig("", filepath.Join(t.TempDir(), "gooru.db"), Overrides{AuthToken: "  "})
	if err == nil || !strings.Contains(err.Error(), "auth token override must not be blank") {
		t.Fatalf("expected blank override error, got %v", err)
	}
}

func TestLoadConfigRejectsNonLoopbackWithoutAuth(t *testing.T) {
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
  completed_ttl: "1h"
`)
	_, err := LoadConfig(path, "", Overrides{})
	if err == nil || !strings.Contains(err.Error(), "refusing unauthenticated non-loopback") {
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
