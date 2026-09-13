package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDatabaseParentCreatesMissingDirectories(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "nested", "library", "gooru.db")
	parent := filepath.Dir(dbPath)

	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("expected parent to be absent before setup, err=%v", err)
	}

	if err := ensureDatabaseParent(dbPath); err != nil {
		t.Fatalf("create database parent: %v", err)
	}

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("stat created parent: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created parent is not a directory: mode=%v", info.Mode())
	}
}
