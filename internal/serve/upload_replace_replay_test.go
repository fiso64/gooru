package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivateReplacementCanReplayAfterActivation(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "item.jpg")
	stagedPath := filepath.Join(dir, ".item.tmp")
	writeReplacementTestFile(t, finalPath, "old")
	writeReplacementTestFile(t, stagedPath, "new")

	first, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("first activation: %v", err)
	}
	second, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("replayed activation: %v", err)
	}
	if !first.hadOriginal || !second.hadOriginal {
		t.Fatalf("hadOriginal = %v/%v, want true/true", first.hadOriginal, second.hadOriginal)
	}
	assertReplacementTestFile(t, finalPath, "new")
	assertReplacementTestFile(t, first.backupPath, "old")
}

func TestActivateReplacementResumesAfterOriginalIsPreserved(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "item.jpg")
	stagedPath := filepath.Join(dir, ".item.tmp")
	backupPath := stagedPath + ".backup"
	writeReplacementTestFile(t, finalPath, "old")
	writeReplacementTestFile(t, stagedPath, "new")
	if err := os.Rename(finalPath, backupPath); err != nil {
		t.Fatalf("preserve original: %v", err)
	}

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("resume activation: %v", err)
	}
	if !replacement.hadOriginal {
		t.Fatal("hadOriginal = false, want true")
	}
	assertReplacementTestFile(t, finalPath, "new")
	assertReplacementTestFile(t, backupPath, "old")
}

func TestActivateReplacementCanReplayWhenOriginalDisappeared(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "item.jpg")
	stagedPath := filepath.Join(dir, ".item.tmp")
	writeReplacementTestFile(t, stagedPath, "new")

	first, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("first activation: %v", err)
	}
	second, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("replayed activation: %v", err)
	}
	if first.hadOriginal || second.hadOriginal {
		t.Fatalf("hadOriginal = %v/%v, want false/false", first.hadOriginal, second.hadOriginal)
	}
	assertReplacementTestFile(t, finalPath, "new")
}

func TestRollbackReplacementIsReplaySafe(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "item.jpg")
	stagedPath := filepath.Join(dir, ".item.tmp")
	writeReplacementTestFile(t, finalPath, "old")
	writeReplacementTestFile(t, stagedPath, "new")

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("activate replacement: %v", err)
	}
	if err := rollbackReplacement(replacement); err != nil {
		t.Fatalf("first rollback: %v", err)
	}
	if err := rollbackReplacement(replacement); err != nil {
		t.Fatalf("replayed rollback: %v", err)
	}
	assertReplacementTestFile(t, finalPath, "old")
	if _, err := os.Stat(replacement.backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup stat after rollback = %v, want not exist", err)
	}
}

func TestCommitReplacementIsReplaySafe(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "item.jpg")
	stagedPath := filepath.Join(dir, ".item.tmp")
	writeReplacementTestFile(t, finalPath, "old")
	writeReplacementTestFile(t, stagedPath, "new")

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("activate replacement: %v", err)
	}
	commitReplacement(replacement)
	commitReplacement(replacement)
	assertReplacementTestFile(t, finalPath, "new")
	if _, err := os.Stat(replacement.backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup stat after commit = %v, want not exist", err)
	}
}

func writeReplacementTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertReplacementTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := string(data); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
