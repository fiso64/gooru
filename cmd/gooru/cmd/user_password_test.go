package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAdminPasswordPreservesEnvironmentValue(t *testing.T) {
	t.Setenv("GOORU_ADMIN_PASSWORD", "  correct horse  ")
	got, err := adminPassword(&cobra.Command{})
	if err != nil {
		t.Fatalf("admin password: %v", err)
	}
	if got != "  correct horse  " {
		t.Fatalf("admin password = %q, want exact environment value", got)
	}
}

func TestAdminPasswordRejectsEmptyEnvironmentValue(t *testing.T) {
	t.Setenv("GOORU_ADMIN_PASSWORD", "")
	_, err := adminPassword(&cobra.Command{})
	if err == nil {
		t.Fatal("admin password unexpectedly accepted an empty environment value")
	}
	if err.Error() != "password is required" {
		t.Fatalf("admin password error = %q, want %q", err, "password is required")
	}
}
