package gooru

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageTargetMetatagsKeepMixedLocationContentNonExternal(t *testing.T) {
	payload := []byte("duplicate-content")
	client, managedFile, managedPath := trackedTestFile(t, payload)
	defer client.Close()

	managedRoot := filepath.Dir(managedPath)
	externalRoot := t.TempDir()
	duplicatePath := filepath.Join(externalRoot, "duplicate.bin")
	if err := os.WriteFile(duplicatePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	duplicateMeta, err := client.hasher.FileMetadata(duplicatePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, managedFile.Hash, duplicatePath, duplicateMeta.Size, duplicateMeta.ModTime.Unix(), filepath.Ext(duplicatePath)); err != nil {
		t.Fatal(err)
	}

	externalOnlyPath := filepath.Join(externalRoot, "external-only.bin")
	if err := os.WriteFile(externalOnlyPath, []byte("external-only-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	externalHash, err := client.hasher.HashFile(externalOnlyPath)
	if err != nil {
		t.Fatal(err)
	}
	externalMeta, err := client.hasher.FileMetadata(externalOnlyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.GetOrCreateContent(client.store, externalHash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, externalHash, externalOnlyPath, externalMeta.Size, externalMeta.ModTime.Unix(), filepath.Ext(externalOnlyPath)); err != nil {
		t.Fatal(err)
	}

	if _, err := client.ReconcileManagedRoots(map[string]string{"main": managedRoot}); err != nil {
		t.Fatal(err)
	}

	for query, want := range map[string]int{
		"@in_target:any":  1,
		"@in_target:main": 1,
		"@external":       1,
		"-@external":      1,
	} {
		got, err := client.CountFilesByQuery(query, false)
		if err != nil {
			t.Fatalf("CountFilesByQuery(%q): %v", query, err)
		}
		if got != want {
			t.Fatalf("CountFilesByQuery(%q)=%d, want %d", query, got, want)
		}
	}

	for query, want := range map[string]int{
		"@in_target:any":  1,
		"@in_target:main": 1,
		"@external":       2,
		"-@external":      1,
	} {
		got, err := client.CountFileLocationsByQuery(query, false)
		if err != nil {
			t.Fatalf("CountFileLocationsByQuery(%q): %v", query, err)
		}
		if got != want {
			t.Fatalf("CountFileLocationsByQuery(%q)=%d, want %d", query, got, want)
		}
	}
}
