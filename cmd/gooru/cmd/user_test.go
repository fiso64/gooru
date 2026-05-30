package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"gooru.local/internal/serve"
)

func TestReadLineSecretUsesSharedReader(t *testing.T) {
	input := bufio.NewReader(bytes.NewBufferString("correct horse\ncorrect horse\n"))
	cmd := &cobra.Command{}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	first, err := readLineSecret(cmd, input, "Password: ")
	if err != nil {
		t.Fatalf("read first secret: %v", err)
	}
	second, err := readLineSecret(cmd, input, "Confirm password: ")
	if err != nil {
		t.Fatalf("read second secret: %v", err)
	}
	if first != "correct horse" || second != "correct horse" {
		t.Fatalf("unexpected secrets %q %q", first, second)
	}
	if got := stderr.String(); got != "Password: Confirm password: " {
		t.Fatalf("unexpected prompts %q", got)
	}
}

func TestPrepareAdminDatabaseSupportsFreshPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "gooru.db")
	store, err := prepareAdminDatabase(dbPath, false)
	if err != nil {
		t.Fatalf("prepare admin database: %v", err)
	}
	defer store.Close()
	if _, err := store.GetHashingStrategy(); err != nil {
		t.Fatalf("expected hashing strategy to be initialized: %v", err)
	}
	authStore := serve.NewAuthStore(store.DB, 0)
	if _, err := authStore.CreateAdmin(context.Background(), "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := authStore.CreateAdmin(context.Background(), "mac", "correct horse"); !errors.Is(err, serve.ErrDuplicateUsername) {
		t.Fatalf("expected duplicate username rejection, got %v", err)
	}
}
