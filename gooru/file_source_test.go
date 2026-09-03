package gooru

import (
	"bytes"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestGetFileInfoForSourceMatchesPlaintextContentIdentity(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatal(err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	content := bytes.Repeat([]byte("protected-source-"), 60000)
	logicalPath := filepath.Join(dir, "library", "media.bin")
	info, status, err := client.GetFileInfoForSource(logicalPath, bytes.NewReader(content), int64(len(content)), 1234)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.StatusNotInDB {
		t.Fatalf("status = %v, want StatusNotInDB", status)
	}
	if info.Path != logicalPath || info.Size != int64(len(content)) || info.ModTime != 1234 || info.Hash == "" {
		t.Fatalf("source info = %+v", info)
	}

	if _, err := client.TagKnownFiles([]types.LocationInfo{{
		Path: logicalPath, Hash: info.Hash, Size: info.Size, ModTime: info.ModTime, Extension: filepath.Ext(logicalPath),
	}}, []string{"source:test"}, nil); err != nil {
		t.Fatal(err)
	}
	same, status, err := client.GetFileInfoForSource(logicalPath, bytes.NewReader(content), int64(len(content)), 5678)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.StatusOK || same.Hash != info.Hash || len(same.Tags) != 1 || same.Tags[0] != "source:test" {
		t.Fatalf("same source status/info = %v %+v", status, same)
	}

	changed := append([]byte(nil), content...)
	// StrategyPartial deliberately samples selected regions. Mutate the first
	// sampled chunk so this assertion tests the algorithm rather than assuming
	// unsampled middle bytes affect the configured content identity.
	changed[0] ^= 0xff
	changedInfo, status, err := client.GetFileInfoForSource(logicalPath, bytes.NewReader(changed), int64(len(changed)), 5678)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.StatusModified || changedInfo.Hash == info.Hash {
		t.Fatalf("changed source status/info = %v %+v", status, changedInfo)
	}
}
