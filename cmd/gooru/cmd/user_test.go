package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
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
	store, err := prepareAdminDatabase(serve.DefaultConfig(dbPath), false)
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

func TestCreateAdminWithPolicyIfMissingIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	store, err := prepareAdminDatabase(serve.DefaultConfig(dbPath), false)
	if err != nil {
		t.Fatalf("prepare admin database: %v", err)
	}
	defer store.Close()

	authStore := serve.NewAuthStore(store.DB, 0)
	created, err := createAdminWithPolicy(context.Background(), authStore, "mac", "correct horse", false)
	if err != nil {
		t.Fatalf("create initial admin: %v", err)
	}
	if !created {
		t.Fatal("expected initial admin to be created")
	}

	created, err = createAdminWithPolicy(context.Background(), authStore, "mac", "different horse", true)
	if err != nil {
		t.Fatalf("ensure existing admin: %v", err)
	}
	if created {
		t.Fatal("existing admin should not be recreated")
	}
	if _, err := authStore.Login(context.Background(), "mac", "correct horse"); err != nil {
		t.Fatalf("original password should remain valid: %v", err)
	}
	if _, err := authStore.Login(context.Background(), "mac", "different horse"); !errors.Is(err, serve.ErrInvalidCredentials) {
		t.Fatalf("existing password must not be reconciled, got %v", err)
	}
}

func TestPrepareAdminDatabasePreservesExistingParentPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir db parent: %v", err)
	}
	dbPath := filepath.Join(dir, "gooru.db")
	store, err := prepareAdminDatabase(serve.DefaultConfig(dbPath), false)
	if err != nil {
		t.Fatalf("prepare admin database: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat db parent: %v", err)
	}
	if got := info.Mode().Perm(); got != 0755 {
		t.Fatalf("db parent mode = %o, want 755", got)
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db file: %v", err)
	}
	if got := dbInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("db file mode = %o, want 600", got)
	}
}
