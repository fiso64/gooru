package serve

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigUploadTargetDefaultTagsValidateDirectives(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: t.TempDir(), DefaultTags: []string{" project:inbox ", "-project:archive"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate default_tags: %v", err)
	}
	if got := cfg.Uploads.Targets[0].DefaultTags[0]; got != "project:inbox" {
		t.Fatalf("trimmed default tag=%q", got)
	}
	cfg.Uploads.Targets[0].DefaultTags = []string{"-"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "default_tags") {
		t.Fatalf("expected default_tags validation error, got %v", err)
	}
}
