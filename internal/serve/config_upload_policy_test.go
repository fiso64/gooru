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
