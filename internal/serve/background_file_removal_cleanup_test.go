package serve

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type canceledRemovalTestLibrary struct {
	emptyLibrary
	files map[string]types.FileInfo
}

func (l *canceledRemovalTestLibrary) PublicFileID(file types.FileInfo) string {
	return file.PublicID
}

func (l *canceledRemovalTestLibrary) GetFileByPublicID(_ context.Context, publicID string) (types.FileInfo, error) {
	file, ok := l.files[publicID]
	if !ok {
		return types.FileInfo{}, ErrNotFound
	}
	return file, nil
}

type fakeDurableRemovalCancellationStore struct {
	operation      core.BackgroundOperationState
	task           core.BackgroundTaskState
	cleanupRequest core.BackgroundTaskRequest
	cancelCalls    int
}

func (f *fakeDurableRemovalCancellationStore) GetBackgroundOperation(operationID string) (core.BackgroundOperationState, bool, error) {
	if f.operation.ID != operationID {
		return core.BackgroundOperationState{}, false, nil
	}
	return f.operation, true, nil
}

func (f *fakeDurableRemovalCancellationStore) GetBackgroundOperationTask(operationID string) (core.BackgroundTaskState, bool, error) {
	if f.operation.ID != operationID {
		return core.BackgroundTaskState{}, false, nil
	}
	return f.task, true, nil
}

func (f *fakeDurableRemovalCancellationStore) CancelBackgroundOperationWithCleanupTask(_ string, request core.BackgroundTaskRequest) (core.BackgroundOperationCancellation, error) {
	f.cancelCalls++
	f.cleanupRequest = request
	return core.BackgroundOperationCancellation{Canceled: true}, nil
}

func TestCancelDurableFileRemovalOperationQueuesDetachedCleanup(t *testing.T) {
	store := &fakeDurableRemovalCancellationStore{
		operation: core.BackgroundOperationState{ID: "operation-delete", Kind: backgroundFileRemovalDeleteOperationKind},
		task:      core.BackgroundTaskState{BackgroundTask: core.BackgroundTask{Kind: backgroundFileRemovalTaskKind}},
	}

	handled, canceled, err := cancelDurableFileRemovalOperation(store, "operation-delete")
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !canceled || store.cancelCalls != 1 {
		t.Fatalf("handled=%v canceled=%v cancel_calls=%d", handled, canceled, store.cancelCalls)
	}
	request := store.cleanupRequest
	if request.OperationID != "" || request.Kind != backgroundFileRemovalCleanupTaskKind || request.SubjectKind != "operation" || request.SubjectID != "operation-delete" || request.ResourceClass != backgroundFileRemovalResourceClass {
		t.Fatalf("unexpected cleanup request: %+v", request)
	}
}

func TestCleanupCanceledFileRemovalRestoresPreCommitBatch(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	stagingPath := filepath.Join(root, ".gooru-delete-cancel-000000")
	if err := os.WriteFile(stagingPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
		"file_a": {PublicID: "file_a", Path: originalPath},
	})

	if err := server.cleanupCanceledFileRemoval(context.Background(), canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}})); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, originalPath, "original")
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging path still exists after restore: %v", err)
	}
}

func TestCleanupCanceledFileRemovalPurgesPostCommitStagingWithoutTouchingReplacement(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	stagingPath := filepath.Join(root, ".gooru-delete-cancel-000000")
	if err := os.WriteFile(stagingPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, nil)

	if err := server.cleanupCanceledFileRemoval(context.Background(), canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}})); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, originalPath, "replacement")
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging path still exists after purge: %v", err)
	}
}

func TestCleanupCanceledFileRemovalReconcilesWholeBatch(t *testing.T) {
	root := t.TempDir()
	firstOriginal := filepath.Join(root, "a.jpg")
	secondOriginal := filepath.Join(root, "b.jpg")
	firstStaging := filepath.Join(root, ".gooru-delete-cancel-000000")
	secondStaging := filepath.Join(root, ".gooru-delete-cancel-000001")
	if err := os.WriteFile(firstStaging, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondStaging, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
		"file_a": {PublicID: "file_a", Path: firstOriginal},
	})

	task := canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{
		{PublicID: "file_a", OriginalPath: firstOriginal, StagingPath: firstStaging},
		{PublicID: "file_b", OriginalPath: secondOriginal, StagingPath: secondStaging},
	})
	if err := server.cleanupCanceledFileRemoval(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, firstOriginal, "first")
	if _, err := os.Stat(secondStaging); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-commit batch staging path still exists: %v", err)
	}
}

func TestCleanupCanceledFileRemovalPreservesStagingOnPathDriftAndCollision(t *testing.T) {
	t.Run("path drift", func(t *testing.T) {
		root := t.TempDir()
		originalPath := filepath.Join(root, "a.jpg")
		driftedPath := filepath.Join(root, "moved.jpg")
		stagingPath := filepath.Join(root, ".gooru-delete-cancel-000000")
		if err := os.WriteFile(stagingPath, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
		server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
			"file_a": {PublicID: "file_a", Path: driftedPath},
		})

		err := server.cleanupCanceledFileRemoval(context.Background(), canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}}))
		if err == nil || !strings.Contains(err.Error(), "managed file path changed") {
			t.Fatalf("expected path drift error, got %v", err)
		}
		assertFileContent(t, stagingPath, "original")
	})

	t.Run("restore collision", func(t *testing.T) {
		root := t.TempDir()
		originalPath := filepath.Join(root, "a.jpg")
		stagingPath := filepath.Join(root, ".gooru-delete-cancel-000000")
		if err := os.WriteFile(stagingPath, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
			"file_a": {PublicID: "file_a", Path: originalPath},
		})

		err := server.cleanupCanceledFileRemoval(context.Background(), canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}}))
		if err == nil || !strings.Contains(err.Error(), "original path is occupied") {
			t.Fatalf("expected restore collision error, got %v", err)
		}
		assertFileContent(t, originalPath, "replacement")
		assertFileContent(t, stagingPath, "original")
	})
}

func TestCleanupCanceledFileRemovalRejectsParentSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o700); err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(album, "a.jpg")
	stagingPath := filepath.Join(album, ".gooru-delete-cancel-000000")
	if err := os.WriteFile(stagingPath, []byte("staged-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	movedAlbum := filepath.Join(root, "album-moved")
	if err := os.Rename(album, movedAlbum); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideStaging := filepath.Join(outside, filepath.Base(stagingPath))
	if err := os.WriteFile(outsideStaging, []byte("outside-victim"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, album); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	server := newCanceledRemovalTestServer(root, nil)

	err := server.cleanupCanceledFileRemoval(context.Background(), canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}}))
	if err == nil || !errors.Is(err, ErrFileNotManaged) {
		t.Fatalf("expected replaced parent to fail managed-path validation, got %v", err)
	}
	assertFileContent(t, outsideStaging, "outside-victim")
	assertFileContent(t, filepath.Join(movedAlbum, filepath.Base(stagingPath)), "staged-original")
}

func TestCleanupCanceledFileRemovalRejectsStagedSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	outsideFile := filepath.Join(t.TempDir(), "outside.jpg")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	stagingPath := filepath.Join(root, ".gooru-delete-cancel-000000")
	if err := os.Symlink(outsideFile, stagingPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
		"file_a": {PublicID: "file_a", Path: originalPath},
	})

	err := server.cleanupCanceledFileRemoval(context.Background(), canceledBatchRemovalTask(t, []backgroundFileRemovalBatchFile{{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}}))
	if err == nil || !strings.Contains(err.Error(), "staging path is a symlink") {
		t.Fatalf("expected staged symlink replacement to fail closed, got %v", err)
	}
	if _, err := os.Lstat(originalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup restored attacker symlink into original path: %v", err)
	}
	assertFileContent(t, outsideFile, "outside")
}

func TestCleanupCanceledFileRemovalSupportsLegacySingleFilePayload(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	stagingDir := filepath.Join(root, ".gooru-delete-cancel")
	stagingPath := filepath.Join(stagingDir, "a.jpg")
	if err := os.Mkdir(stagingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagingPath, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
		"file_a": {PublicID: "file_a", Path: originalPath},
	})
	input := backgroundFileRemovalInput{Mode: "delete", PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	task := core.BackgroundTask{Kind: backgroundFileRemovalTaskKind, SubjectKind: "file", SubjectID: "file_a", InputKey: string(encoded)}

	if err := server.cleanupCanceledFileRemoval(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, originalPath, "legacy")
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy staging directory still exists: %v", err)
	}
}

func newCanceledRemovalTestServer(root string, files map[string]types.FileInfo) *Server {
	cfg := DefaultConfig(filepath.Join(root, "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: root}}
	return NewServerWithLibrary(cfg, &canceledRemovalTestLibrary{files: files})
}

func canceledBatchRemovalTask(t *testing.T, files []backgroundFileRemovalBatchFile) core.BackgroundTask {
	t.Helper()
	encoded, err := json.Marshal(backgroundFileRemovalBatchInput{Version: backgroundFileRemovalBatchVersion, Mode: "delete", Files: files})
	if err != nil {
		t.Fatal(err)
	}
	return core.BackgroundTask{Kind: backgroundFileRemovalTaskKind, SubjectKind: "file_batch", SubjectID: "selection", InputKey: string(encoded)}
}
