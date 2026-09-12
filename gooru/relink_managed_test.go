package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestRelinkDoesNotMoveLiveManagedLocationToDuplicate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	contents := []byte("same managed contents")
	physicalPath := filepath.Join(t.TempDir(), "managed.bin")
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

	logicalPath := filepath.Join(t.TempDir(), "library", "upload.bin") // deliberately absent on disk
	if _, err := client.store.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, hash, logicalPath, info.Size, info.ModTime.Unix(), ".bin"); err != nil {
		t.Fatal(err)
	}
	managed, err := client.store.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, managed.ID, physicalPath); err != nil {
		t.Fatal(err)
	}

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
