package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigUploadTargetAddedAtStrategyDefaultsAndValidates(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: t.TempDir()}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate default target strategy: %v", err)
	}
	if got := cfg.Uploads.Targets[0].AddedAtStrategy; got != "queue" {
		t.Fatalf("default added_at_strategy=%q want queue", got)
	}
	cfg.Uploads.Targets[0].AddedAtStrategy = "random"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "added_at_strategy") {
		t.Fatalf("expected added_at_strategy validation error, got %v", err)
	}
}

func TestLoadConfigDefaultsAreValid(t *testing.T) {
	testDefaultUploadRoot(t)
	cfg, err := LoadConfig("", filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig default failed: %v", err)
	}
	if cfg.Server.Listen != DefaultListenAddress {
		t.Fatalf("unexpected listen address %q", cfg.Server.Listen)
	}
	if !cfg.Uploads.Enabled || len(cfg.Uploads.Targets) != 1 {
		t.Fatalf("authenticated defaults should provide one upload target: %+v", cfg.Uploads)
	}
	if !filepath.IsAbs(cfg.Uploads.Targets[0].Path) {
		t.Fatalf("default upload target must be absolute: %+v", cfg.Uploads.Targets[0])
	}
	if cfg.Server.ExposePaths {
		t.Fatal("server.expose_paths should default false")
	}
	if cfg.Server.FrontendDir == "" {
		t.Fatal("server.frontend_dir should point at the static frontend build by default")
	}
	if !cfg.Uploads.PreserveModTime {
		t.Fatal("uploads.preserve_modtime should default true")
	}
}

func TestUIPaginationConfigDefaultsAndValidates(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.UI.PaginationMode != "infinite" || cfg.UI.ItemsPerPage != 60 {
		t.Fatalf("unexpected pagination defaults: mode=%q items=%d", cfg.UI.PaginationMode, cfg.UI.ItemsPerPage)
	}
	cfg.UI.PaginationMode = "paged"
	cfg.UI.ItemsPerPage = MaxPageLimit
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate paged UI config: %v", err)
	}
	cfg.UI.PaginationMode = "pages"
	cfg.UI.ItemsPerPage = MaxPageLimit + 1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ui.pagination_mode") || !strings.Contains(err.Error(), "ui.items_per_page") {
		t.Fatalf("expected pagination validation errors, got %v", err)
	}
}

func TestLoadConfigEncryptionKeyFile(t *testing.T) {
	testDefaultUploadRoot(t)
	keyPath := filepath.Join(t.TempDir(), "encryption.key")
	if err := os.WriteFile(keyPath, []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"), 0600); err != nil {
		t.Fatalf("write encryption key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "encryption:\n  enabled: true\n  key_file: "+keyPath)

	cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig with encryption.key_file failed: %v", err)
	}
	if cfg.Encryption.KeyFile != keyPath {
		t.Fatalf("unexpected encryption key file %q", cfg.Encryption.KeyFile)
	}
	if len(cfg.Encryption.Key) != 32 {
		t.Fatalf("expected resolved 32-byte key, got %d bytes", len(cfg.Encryption.Key))
	}
}

func TestLoadConfigEncryptionKeyFileRejectsEnvironmentConflict(t *testing.T) {
	t.Setenv("GOORU_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	keyPath := filepath.Join(t.TempDir(), "encryption.key")
	if err := os.WriteFile(keyPath, []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"), 0600); err != nil {
		t.Fatalf("write encryption key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "encryption:\n  enabled: true\n  key_file: "+keyPath)

	_, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "configure exactly one encryption key source") {
		t.Fatalf("expected encryption key source conflict, got %v", err)
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
		t.Fatalf("expected upload target validation error, got %v", err)
	}
}

func TestLoadConfigRejectsInvalidUploadTargets(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{
		{ID: "default", Name: "Default", Path: filepath.Join(t.TempDir(), "uploads")},
		{ID: "default", Name: "Duplicate", Path: filepath.Join(t.TempDir(), "other")},
		{ID: "../bad", Name: "Bad", Path: "relative"},
	}
	err := cfg.Validate()
	if err == nil ||
		!strings.Contains(err.Error(), "duplicated") ||
		!strings.Contains(err.Error(), "must contain only") ||
		!strings.Contains(err.Error(), "path must be absolute") {
		t.Fatalf("expected upload target validation errors, got %v", err)
	}
}

func TestLoadConfigNormalizesUploadTargets(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: " default ", Name: " Default ", Path: " " + filepath.Join(t.TempDir(), "uploads") + " "}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}
	target := cfg.Uploads.Targets[0]
	if target.ID != "default" || target.Name != "Default" || target.Path != strings.TrimSpace(target.Path) {
		t.Fatalf("upload target was not normalized: %+v", target)
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

func TestLoadConfigRejectsRemovedUploadMaxQueued(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, `
uploads:
  max_queued: 100
`)
	_, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "field max_queued not found") {
		t.Fatalf("expected removed uploads.max_queued field to be rejected, got %v", err)
	}
}

func TestDefaultYAMLParses(t *testing.T) {
	testDefaultUploadRoot(t)
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
