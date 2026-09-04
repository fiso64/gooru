package serve

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigUploadTargetDefaultTagsValidateOrdinaryTags(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: t.TempDir(), DefaultTags: []string{" project:inbox ", "source:upload", " -project:archive "}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate default_tags: %v", err)
	}
	if got := cfg.Uploads.Targets[0].DefaultTags; len(got) != 3 || got[0] != "project:inbox" || got[1] != "source:upload" || got[2] != "-project:archive" {
		t.Fatalf("trimmed default tags=%v", got)
	}
	cfg.Uploads.Targets[0].DefaultTags = []string{"bad tag"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "default_tags") {
		t.Fatalf("expected default_tags validation error, got %v", err)
	}
}
