package gooru

import (
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestFileRegistrationHashesForLocationUpsertsDetectsNewKnownContentLocation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	dir := t.TempDir()
	existing := registerKnownContentWithoutHooks(t, client, filepath.Join(dir, "existing.jpg"), "shared-hash")

	tx, err := client.store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	newLocation := types.LocationInfo{Path: filepath.Join(dir, "duplicate.jpg"), Hash: existing.Hash, Size: 1, ModTime: 2}
	hashes, err := fileRegistrationHashesForLocationUpserts(tx, map[string]types.LocationInfo{newLocation.Path: newLocation})
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 1 || hashes[0] != existing.Hash {
		t.Fatalf("registration hashes = %#v, want [%q]", hashes, existing.Hash)
	}
}

func TestFileRegistrationHashesForLocationUpsertsSkipsUnchangedLocation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	path := filepath.Join(t.TempDir(), "existing.jpg")
	existing := registerKnownContentWithoutHooks(t, client, path, "existing-hash")

	tx, err := client.store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	hashes, err := fileRegistrationHashesForLocationUpserts(tx, map[string]types.LocationInfo{path: existing})
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 0 {
		t.Fatalf("unchanged location produced registration hashes: %#v", hashes)
	}
}

func TestNewLocationOfKnownContentEnqueuesMetadataSweep(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	dir := t.TempDir()
	existing := registerKnownContentWithoutHooks(t, client, filepath.Join(dir, "existing.jpg"), "shared-hash")
	newLocation := types.LocationInfo{Path: filepath.Join(dir, "duplicate.jpg"), Hash: existing.Hash, Size: 1, ModTime: 2}
	if _, err := client.TagKnownFiles([]types.LocationInfo{newLocation}, nil, nil); err != nil {
		t.Fatalf("register new location for known content: %v", err)
	}

	var count int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE kind = ?`, BackgroundMediaMetadataSweepTaskKind).Scan(&count); err != nil {
		t.Fatalf("count metadata sweep tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("metadata sweep task count = %d, want 1 for new location", count)
	}
}

func TestKnownHashNewLocationSeparatesMetadataWakeFromParentOperation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	dir := t.TempDir()
	existing := registerKnownContentWithoutHooks(t, client, filepath.Join(dir, "existing.jpg"), "shared-upload-hash")
	operation, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "upload.import", Visible: true, ProgressTotal: 63})
	if err != nil {
		t.Fatalf("create upload operation: %v", err)
	}
	newLocation := types.LocationInfo{Path: filepath.Join(dir, "duplicate.jpg"), Hash: existing.Hash, Size: 1, ModTime: 2}
	_, err = client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState([]types.LocationInfo{newLocation}, nil, nil, func(int) (BackgroundOperationTransactionState, error) {
		return BackgroundOperationTransactionState{OperationID: operation.ID}, nil
	}, nil)
	if err != nil {
		t.Fatalf("register known content at new upload location: %v", err)
	}

	var count int
	var metadataOperationID string
	if err := client.store.DB.QueryRow(`SELECT count(*), min(operation_id) FROM background_tasks WHERE kind = ?`, BackgroundMediaMetadataSweepTaskKind).Scan(&count, &metadataOperationID); err != nil {
		t.Fatalf("inspect metadata wake: %v", err)
	}
	if count != 1 || metadataOperationID == "" || metadataOperationID == operation.ID {
		t.Fatalf("metadata wakes = %d on operation %q, want one separate from upload %q", count, metadataOperationID, operation.ID)
	}
	var visible int
	if err := client.store.DB.QueryRow(`SELECT visible FROM background_operations WHERE id = ? AND kind = ?`, metadataOperationID, BackgroundMediaMetadataSweepOperationKind).Scan(&visible); err != nil {
		t.Fatalf("inspect metadata operation: %v", err)
	}
	if visible != 1 {
		t.Fatalf("upload-triggered metadata operation visible = %d, want visible auxiliary", visible)
	}
	var uploadChildren int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = ?`, operation.ID).Scan(&uploadChildren); err != nil {
		t.Fatalf("count upload children: %v", err)
	}
	if uploadChildren != 0 {
		t.Fatalf("upload operation gained %d metadata children, want 0", uploadChildren)
	}
}
