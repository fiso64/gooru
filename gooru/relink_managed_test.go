package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func addManagedTestLocation(t *testing.T, client *Client, logicalPath, physicalPath string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(physicalPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := client.hasher.HashFile(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.hasher.FileMetadata(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.Exec(`INSERT OR IGNORE INTO contents (hash) VALUES (?)`, hash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, hash, logicalPath, info.Size, info.ModTime.Unix(), filepath.Ext(logicalPath)); err != nil {
		t.Fatal(err)
	}
	managed, err := client.store.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, managed.ID, physicalPath); err != nil {
		t.Fatal(err)
	}
}

func newManagedRelinkClient(t *testing.T) *Client {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestRelinkDoesNotMoveLiveManagedLocationToDuplicate(t *testing.T) {
	client := newManagedRelinkClient(t)
	contents := []byte("same managed contents")
	physicalPath := filepath.Join(t.TempDir(), "managed.bin")
	logicalPath := filepath.Join(t.TempDir(), "library", "upload.bin") // deliberately absent on disk
	addManagedTestLocation(t, client, logicalPath, physicalPath, contents)

	scanDir := t.TempDir()
	duplicatePath := filepath.Join(scanDir, "duplicate.bin")
	if err := os.WriteFile(duplicatePath, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := client.Relink([]string{scanDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProposedMoves) != 0 {
		t.Fatalf("managed location was treated as missing move source: %+v", result.ProposedMoves)
	}
	if len(result.ProposedDeletes) != 0 {
		t.Fatalf("unexpected deletes: %+v", result.ProposedDeletes)
	}
	if len(result.ProposedAdds) != 1 || result.ProposedAdds[0].Path != duplicatePath {
		t.Fatalf("adds = %+v, want duplicate %q", result.ProposedAdds, duplicatePath)
	}
}

func TestRelinkDoesNotDeleteLiveManagedLocationInScope(t *testing.T) {
	client := newManagedRelinkClient(t)
	scanDir := t.TempDir()
	logicalPath := filepath.Join(scanDir, "managed", "upload.bin") // deliberately absent on disk
	physicalPath := filepath.Join(t.TempDir(), "managed.bin")
	addManagedTestLocation(t, client, logicalPath, physicalPath, []byte("managed contents"))

	result, err := client.Relink([]string{scanDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProposedMoves) != 0 || len(result.ProposedAdds) != 0 || len(result.ProposedDeletes) != 0 {
		t.Fatalf("live managed location produced relink changes: %+v", result)
	}
}

func TestNeedsRelinkUsesManagedPhysicalSource(t *testing.T) {
	client := newManagedRelinkClient(t)
	scanDir := t.TempDir()
	logicalPath := filepath.Join(scanDir, "managed", "upload.bin") // deliberately absent on disk
	physicalPath := filepath.Join(t.TempDir(), "managed.bin")
	addManagedTestLocation(t, client, logicalPath, physicalPath, []byte("managed contents"))

	for _, verifyHash := range []bool{false, true} {
		needsRelink, err := client.NeedsRelink([]string{scanDir}, verifyHash)
		if err != nil {
			t.Fatal(err)
		}
		if needsRelink {
			t.Fatalf("NeedsRelink(alwaysVerifyHash=%v) = true for unchanged managed source", verifyHash)
		}
	}
}

func TestRelinkRepairsMovedManagedPhysicalSource(t *testing.T) {
	client := newManagedRelinkClient(t)
	scanDir := t.TempDir()
	logicalPath := filepath.Join(scanDir, "managed", "upload.bin") // canonical identity remains absent on disk
	oldPhysicalPath := filepath.Join(t.TempDir(), "managed.bin")
	addManagedTestLocation(t, client, logicalPath, oldPhysicalPath, []byte("managed contents"))

	before, err := client.store.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	newPhysicalPath := filepath.Join(scanDir, "moved.bin")
	if err := os.Rename(oldPhysicalPath, newPhysicalPath); err != nil {
		t.Fatal(err)
	}

	result, err := client.Relink([]string{scanDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProposedMoves) != 1 {
		t.Fatalf("moves = %+v, want one managed-source repair", result.ProposedMoves)
	}
	move := result.ProposedMoves[0]
	if move.OldPath != logicalPath || move.NewLocation.Path != newPhysicalPath {
		t.Fatalf("move = %+v, want %q -> %q", move, logicalPath, newPhysicalPath)
	}

	if _, err := client.ApplyRelinkChanges(result); err != nil {
		t.Fatal(err)
	}
	after, err := client.store.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatalf("logical identity was not preserved: %v", err)
	}
	if after.ID != before.ID {
		t.Fatalf("location id changed from %d to %d", before.ID, after.ID)
	}
	source, err := client.store.GetLocationSourceByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if source.StoragePath != newPhysicalPath {
		t.Fatalf("physical path = %q, want %q", source.StoragePath, newPhysicalPath)
	}

	needsRelink, err := client.NeedsRelink([]string{scanDir}, true)
	if err != nil {
		t.Fatal(err)
	}
	if needsRelink {
		t.Fatal("managed source still needs relink after physical-path repair")
	}

	result, err = client.Relink([]string{scanDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProposedMoves) != 0 || len(result.ProposedAdds) != 0 || len(result.ProposedDeletes) != 0 {
		t.Fatalf("repaired managed source produced follow-up relink changes: %+v", result)
	}
}
