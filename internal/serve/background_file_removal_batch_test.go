package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestBackgroundFileRemovalBatchTaskUsesOneFlatStagingTargetPerFile(t *testing.T) {
	root := t.TempDir()
	files := []types.FileInfo{
		{ID: 1, PublicID: "file_a", Path: filepath.Join(root, "a.jpg")},
		{ID: 2, PublicID: "file_b", Path: filepath.Join(root, "b.jpg")},
	}
	for _, file := range files {
		if err := os.WriteFile(file.Path, []byte(file.PublicID), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig(filepath.Join(root, "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: root}}
	server := NewServerWithLibrary(cfg, emptyLibrary{})

	task, err := server.backgroundFileRemovalBatchTask("delete", files)
	if err != nil {
		t.Fatal(err)
	}
	if task.SubjectKind != "file_batch" || task.SubjectID != "selection" || task.DedupeKey != "batch" {
		t.Fatalf("unexpected batch task identity: %+v", task)
	}
	var input backgroundFileRemovalBatchInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		t.Fatal(err)
	}
	if input.Version != backgroundFileRemovalBatchVersion || input.Mode != "delete" || len(input.Files) != len(files) {
		t.Fatalf("unexpected batch input: %+v", input)
	}
	for index, item := range input.Files {
		expectedPublicID := server.publicFileID(files[index])
		if item.PublicID != expectedPublicID || item.OriginalPath != files[index].Path {
			t.Fatalf("unexpected batch item %d: got %+v, want public_id=%q path=%q", index, item, expectedPublicID, files[index].Path)
		}
		if filepath.Dir(item.StagingPath) != root || !strings.HasPrefix(filepath.Base(item.StagingPath), ".gooru-delete-") {
			t.Fatalf("staging path is not a hidden sibling: %q", item.StagingPath)
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, ".gooru-delete-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("preparing a task must not create staging directories/files: %v", matches)
	}
}

func TestBackgroundFileRemovalBatchDeletesMultipleFilesInOneTask(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	page := listTestFiles(t, server, "kind:image", 2)
	if len(page.Files) < 2 {
		t.Fatalf("test fixture needs two images, got %d", len(page.Files))
	}
	files := make([]types.FileInfo, 0, 2)
	roots := make(map[string]struct{})
	for _, item := range page.Files[:2] {
		file, err := server.getFileByPublicID(context.Background(), item.ID)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		roots[filepath.Dir(fileStoragePath(file))] = struct{}{}
	}
	server.cfg.Uploads.Targets = nil
	index := 0
	for root := range roots {
		server.cfg.Uploads.Targets = append(server.cfg.Uploads.Targets, UploadTarget{ID: fmt.Sprintf("managed-%d", index), Name: "Managed", Path: root})
		index++
	}

	request, err := server.backgroundFileRemovalBatchTask("delete", files)
	if err != nil {
		t.Fatal(err)
	}
	task := core.BackgroundTask{
		ID:          "task-batch",
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	}
	if err := server.backgroundFileRemovalHandler(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if _, err := os.Stat(fileStoragePath(file)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %q to be removed, stat err=%v", fileStoragePath(file), err)
		}
		if _, err := server.getFileByPublicID(context.Background(), server.publicFileID(file)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected %q to be untracked, err=%v", server.publicFileID(file), err)
		}
	}
	for root := range roots {
		matches, err := filepath.Glob(filepath.Join(root, ".gooru-delete-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("batch cleanup left staging files: %v", matches)
		}
	}
}

type replayBatchRemovalLibrary struct {
	emptyLibrary
	deleteCalls int
	lookupCalls int
	deletedIDs  []string
}

func (l *replayBatchRemovalLibrary) PublicFileID(file types.FileInfo) string {
	return file.PublicID
}

func (l *replayBatchRemovalLibrary) GetFileByPublicID(context.Context, string) (types.FileInfo, error) {
	l.lookupCalls++
	return types.FileInfo{}, errors.New("unexpected public-id lookup during post-commit replay")
}

func (l *replayBatchRemovalLibrary) DeleteFileByPublicID(context.Context, string) (bool, error) {
	return false, errors.New("unexpected single-file delete during batch replay")
}

func (l *replayBatchRemovalLibrary) DeleteFilesByPublicIDs(_ context.Context, publicIDs []string) (int, error) {
	l.deleteCalls++
	l.deletedIDs = append([]string(nil), publicIDs...)
	// Zero rows means a previous attempt already committed the database removal.
	return 0, nil
}

func TestBackgroundFileRemovalBatchReplayAfterDatabaseCommitPreservesReplacement(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	stagingPath := filepath.Join(root, ".gooru-delete-replay-000000")
	if err := os.WriteFile(stagingPath, []byte("staged-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	library := &replayBatchRemovalLibrary{}
	server := NewServerWithLibrary(DefaultConfig(filepath.Join(root, "gooru.db")), library)
	files := []backgroundFileRemovalBatchFile{{
		PublicID:     "file_a",
		OriginalPath: originalPath,
		StagingPath:  stagingPath,
	}}
	if err := server.resumeManagedFileDeletionBatch(context.Background(), files, []string{"file_a"}); err != nil {
		t.Fatal(err)
	}

	replacement, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read replacement after replay: %v", err)
	}
	if string(replacement) != "replacement" {
		t.Fatalf("post-commit replay changed replacement content: %q", replacement)
	}
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected persisted staging file to be cleaned, stat err=%v", err)
	}
	if library.lookupCalls != 0 {
		t.Fatalf("post-commit replay unexpectedly resolved removed public IDs %d times", library.lookupCalls)
	}
	if library.deleteCalls != 1 || len(library.deletedIDs) != 1 || library.deletedIDs[0] != "file_a" {
		t.Fatalf("unexpected batch delete replay: calls=%d ids=%v", library.deleteCalls, library.deletedIDs)
	}
}
