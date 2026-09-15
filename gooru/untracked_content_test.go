package gooru

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestContentCentricInspectionRecognizesKnownUntaggedContent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	payload := []byte("known content without tags")
	dir := t.TempDir()
	trackedPath := filepath.Join(dir, "tracked.bin")
	if err := os.WriteFile(trackedPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TagFiles([]string{trackedPath}, []string{"temporary"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UntagFiles([]string{trackedPath}, []string{"temporary"}, nil, false); err != nil {
		t.Fatal(err)
	}

	duplicatePath := filepath.Join(dir, "duplicate.bin")
	if err := os.WriteFile(duplicatePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	tags, status, err := client.GetTagsForFile(duplicatePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.StatusUntrackedContent {
		t.Fatalf("GetTagsForFile status = %v, want StatusUntrackedContent", status)
	}
	if len(tags) != 0 {
		t.Fatalf("GetTagsForFile tags = %v, want none", tags)
	}

	info, status, err := client.GetFileInfoForFile(duplicatePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.StatusUntrackedContent {
		t.Fatalf("GetFileInfoForFile status = %v, want StatusUntrackedContent", status)
	}
	if len(info.Tags) != 0 {
		t.Fatalf("GetFileInfoForFile tags = %v, want none", info.Tags)
	}

	sourcePath := filepath.Join(dir, "source-only.bin")
	info, status, err = client.GetFileInfoForSource(sourcePath, bytes.NewReader(payload), int64(len(payload)), 123)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.StatusUntrackedContent {
		t.Fatalf("GetFileInfoForSource status = %v, want StatusUntrackedContent", status)
	}
	if len(info.Tags) != 0 {
		t.Fatalf("GetFileInfoForSource tags = %v, want none", info.Tags)
	}
}
