package serve

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestManagedRemovalResolvesPhysicalStoragePath(t *testing.T) {
	managedRoot := t.TempDir()
	physicalPath := filepath.Join(managedRoot, "stored.jpg")
	if err := os.WriteFile(physicalPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	logicalPath := filepath.Join(t.TempDir(), "logical", "stored.jpg")
	file := types.FileInfo{PublicID: "managed-file", Path: logicalPath, StoragePath: physicalPath}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: managedRoot}}
	server := NewServerWithLibrary(cfg, emptyLibrary{})

	if server.canDeleteFilePath(file.Path) {
		t.Fatalf("logical path outside upload target must not pass managed deletion policy: %q", file.Path)
	}
	if !server.canDeleteFilePath(fileStoragePath(file)) {
		t.Fatalf("physical managed storage path should pass deletion policy: %q", physicalPath)
	}

	task, err := server.backgroundFileRemovalBatchTask("delete", []types.FileInfo{file})
	if err != nil {
		t.Fatal(err)
	}
	var input backgroundFileRemovalBatchInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		t.Fatal(err)
	}
	if len(input.Files) != 1 || input.Files[0].OriginalPath != physicalPath {
		t.Fatalf("managed delete task used wrong backing path: %+v", input.Files)
	}
}

func TestMixedRemovalTaskDeletesManagedAndOnlyUntracksExternal(t *testing.T) {
	managedRoot := t.TempDir()
	managedPath := filepath.Join(managedRoot, "managed.jpg")
	externalPath := filepath.Join(t.TempDir(), "external.jpg")
	for _, path := range []string{managedPath, externalPath} {
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: managedRoot}}
	library := &replayBatchRemovalLibrary{
		lookupFile: types.FileInfo{PublicID: "managed", Path: managedPath},
	}
	server := NewServerWithLibrary(cfg, library)

	request, err := server.backgroundFileRemovalBatchTask("delete_or_untrack", []types.FileInfo{
		{PublicID: "managed", Path: managedPath},
		{PublicID: "external", Path: externalPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	var input backgroundFileRemovalBatchInput
	if err := json.Unmarshal([]byte(request.InputKey), &input); err != nil {
		t.Fatal(err)
	}
	if input.Version != backgroundFileRemovalMixedBatchVersion || input.Mode != "delete_or_untrack" || len(input.Files) != 2 {
		t.Fatalf("unexpected mixed removal payload: %+v", input)
	}
	if input.Files[0].OriginalPath != managedPath || input.Files[0].StagingPath == "" {
		t.Fatalf("managed file was not staged for deletion: %+v", input.Files[0])
	}
	if input.Files[1].OriginalPath != "" || input.Files[1].StagingPath != "" {
		t.Fatalf("external file must only be untracked: %+v", input.Files[1])
	}

	task := core.BackgroundTask{
		ID:          "task-mixed",
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	}
	if err := server.backgroundFileRemovalHandler(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("managed file must be deleted, stat err=%v", err)
	}
	if got, err := os.ReadFile(externalPath); err != nil || string(got) != "data" {
		t.Fatalf("external file must remain untouched, content=%q err=%v", got, err)
	}
	if library.deleteCalls != 1 || len(library.deletedIDs) != 2 || library.deletedIDs[0] != "managed" || library.deletedIDs[1] != "external" {
		t.Fatalf("mixed removal must untrack both files in one batch: calls=%d ids=%v", library.deleteCalls, library.deletedIDs)
	}
}
