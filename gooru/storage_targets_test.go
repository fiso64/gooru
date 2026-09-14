package gooru

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReconcileManagedRootsMaintainsTargetCatalogAndMembership(t *testing.T) {
	client, file, oldPath := trackedTestFile(t, []byte("parent-content"))
	defer client.Close()

	parentRoot := filepath.Dir(oldPath)
	nestedRoot := filepath.Join(parentRoot, "nested")
	emptyRoot := filepath.Join(t.TempDir(), "empty")
	for _, dir := range []string{nestedRoot, emptyRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if moved, err := client.ReconcileManagedRoots(map[string]string{
		"parent": parentRoot,
		"nested": nestedRoot,
		"empty":  emptyRoot,
	}); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	} else if moved != 0 {
		t.Fatalf("initial reconcile moved=%d, want 0", moved)
	}

	var targetCount int
	if err := client.store.QueryRow(`SELECT COUNT(*) FROM managed_storage_targets`).Scan(&targetCount); err != nil {
		t.Fatal(err)
	}
	if targetCount != 3 {
		t.Fatalf("target catalog count=%d, want 3", targetCount)
	}
	var memberships int
	if err := client.store.QueryRow(`SELECT COUNT(*) FROM managed_storage_target_locations WHERE location_id = ?`, file.ID).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if memberships != 1 {
		t.Fatalf("existing parent location memberships=%d, want 1", memberships)
	}
	var emptyMemberships int
	if err := client.store.QueryRow(`SELECT COUNT(*) FROM managed_storage_target_locations WHERE target_id = 'empty'`).Scan(&emptyMemberships); err != nil {
		t.Fatal(err)
	}
	if emptyMemberships != 0 {
		t.Fatalf("empty target memberships=%d, want 0", emptyMemberships)
	}

	nestedPath := filepath.Join(nestedRoot, "new.bin")
	if err := os.WriteFile(nestedPath, []byte("nested-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := client.hasher.HashFile(nestedPath)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := client.hasher.FileMetadata(nestedPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.GetOrCreateContent(client.store, hash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, hash, nestedPath, meta.Size, meta.ModTime.Unix(), filepath.Ext(nestedPath)); err != nil {
		t.Fatal(err)
	}
	nestedFile, err := client.GetFileInfoByPath(nestedPath)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := client.store.Query(`SELECT target_id FROM managed_storage_target_locations WHERE location_id = ? ORDER BY target_id`, nestedFile.ID)
	if err != nil {
		t.Fatal(err)
	}
	var gotTargets []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		gotTargets = append(gotTargets, id)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotTargets, []string{"nested", "parent"}) {
		t.Fatalf("new nested location targets=%v, want [nested parent]", gotTargets)
	}

	if _, err := client.ReconcileManagedRoots(map[string]string{}); err != nil {
		t.Fatalf("clear targets: %v", err)
	}
	for name, query := range map[string]string{
		"target catalog": `SELECT COUNT(*) FROM managed_storage_targets`,
		"memberships":   `SELECT COUNT(*) FROM managed_storage_target_locations`,
		"root metadata": `SELECT COUNT(*) FROM meta WHERE key LIKE 'managed_upload_root:%'`,
	} {
		var count int
		if err := client.store.QueryRow(query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s count=%d after clearing targets, want 0", name, count)
		}
	}

	newRoot := filepath.Join(t.TempDir(), "reused-id")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if moved, err := client.ReconcileManagedRoots(map[string]string{"parent": newRoot}); err != nil {
		t.Fatalf("reconcile reused target id: %v", err)
	} else if moved != 0 {
		t.Fatalf("reused target id moved=%d, want 0 after stale metadata cleanup", moved)
	}
	stored, err := client.GetFileInfoByLocationID(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Path != oldPath {
		t.Fatalf("reused target id rebased old location to %q, want %q", stored.Path, oldPath)
	}
}
