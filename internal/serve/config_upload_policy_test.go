package serve

import (
	"path/filepath"
	"testing"
)

func TestUploadConflictPolicyConfigAcceptsSupportedPolicies(t *testing.T) {
	for _, policy := range []string{"skip", "rename", "replace", "error"} {
		t.Run(policy, func(t *testing.T) {
			cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
			cfg.Uploads.ConflictPolicy = policy
			if err := cfg.Validate(); err != nil {
				t.Fatalf("expected uploads.conflict_policy=%q to validate: %v", policy, err)
			}
		})
	}
}

func TestDefaultUploadConflictPolicyIsRename(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.Uploads.ConflictPolicy != "rename" {
		t.Fatalf("expected default uploads.conflict_policy=rename, got %q", cfg.Uploads.ConflictPolicy)
	}
	policy, err := uploadConflictPolicy("", "")
	if err != nil {
		t.Fatalf("resolve omitted conflict policy: %v", err)
	}
	if policy != "rename" {
		t.Fatalf("expected omitted conflict policy to resolve to rename, got %q", policy)
	}
}
