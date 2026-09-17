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
