package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigDirsArePrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dbPath, err := GetDBPath()
	if err != nil {
		t.Fatalf("GetDBPath: %v", err)
	}
	assertMode(t, filepath.Dir(dbPath), 0700)

	runDir, err := GetRunDirPath()
	if err != nil {
		t.Fatalf("GetRunDirPath: %v", err)
	}
	assertMode(t, runDir, 0700)
	assertMode(t, filepath.Dir(runDir), 0700)
}

func TestGetDBPathRepairsExistingPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "gooru")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	dbPath, err := GetDBPath()
	if err != nil {
		t.Fatalf("GetDBPath: %v", err)
	}
	assertMode(t, filepath.Dir(dbPath), 0700)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %o, want %o", path, got, want)
	}
}
