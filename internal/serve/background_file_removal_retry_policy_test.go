package serve

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestBackgroundFileRemovalDeleteRetryPolicyUsesTerminalCompensation(t *testing.T) {
	input := backgroundFileRemovalBatchInput{
		Version: backgroundFileRemovalBatchVersion,
		Mode:    "delete",
		Files: []backgroundFileRemovalBatchFile{{
			PublicID:     "file_a",
			OriginalPath: "/uploads/a.jpg",
			StagingPath:  "/uploads/.gooru-delete-token-000000",
		}},
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request := core.BackgroundTaskRequest{
		DedupeKey:     "batch",
		Kind:          backgroundFileRemovalTaskKind,
		SubjectKind:   "file_batch",
		SubjectID:     "selection",
		InputKey:      string(encoded),
		ResourceClass: backgroundFileRemovalResourceClass,
		MaxAttempts:   5,
	}

	request, err = backgroundFileRemovalApplyDeleteRetryPolicy(request)
	if err != nil {
		t.Fatal(err)
	}
	if request.MaxAttempts != 1 {
		t.Fatalf("delete attempts = %d, want 1", request.MaxAttempts)
	}
	cleanup := request.TerminalFailureCleanup
	if cleanup == nil {
		t.Fatal("delete task is missing terminal compensation")
	}
	if cleanup.Kind != backgroundFileRemovalCleanupTaskKind || cleanup.SubjectKind != "file_batch" || cleanup.SubjectID != "selection" {
		t.Fatalf("unexpected terminal cleanup identity: %+v", cleanup)
	}
	if cleanup.InputKey != request.InputKey {
		t.Fatal("terminal cleanup did not preserve immutable delete payload")
	}
	if cleanup.ResourceClass != backgroundFileRemovalResourceClass || cleanup.Priority != backgroundFileRemovalCleanupPriority || cleanup.MaxAttempts != 5 {
		t.Fatalf("unexpected terminal cleanup scheduling: %+v", cleanup)
	}
}

func TestTerminalFileRemovalCleanupRestoresPreCommitStaging(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	stagingPath := filepath.Join(root, ".gooru-delete-terminal-000000")
	if err := os.WriteFile(stagingPath, []byte("staged-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
		"file_a": {PublicID: "file_a", Path: originalPath},
	})
	task := terminalRemovalCleanupTask(t, backgroundFileRemovalBatchFile{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath})

	if err := server.backgroundFileRemovalCleanupHandler(nil)(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, originalPath, "staged-original")
	if _, err := os.Lstat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal cleanup left staging path after restore: %v", err)
	}
}

func TestTerminalFileRemovalCleanupPurgesPostCommitStaging(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "a.jpg")
	stagingPath := filepath.Join(root, ".gooru-delete-terminal-000000")
	if err := os.WriteFile(stagingPath, []byte("staged-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, nil)
	task := terminalRemovalCleanupTask(t, backgroundFileRemovalBatchFile{PublicID: "file_a", OriginalPath: originalPath, StagingPath: stagingPath})

	if err := server.backgroundFileRemovalCleanupHandler(nil)(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, originalPath, "replacement")
	if _, err := os.Lstat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal cleanup left committed staging path: %v", err)
	}
}

func terminalRemovalCleanupTask(t *testing.T, file backgroundFileRemovalBatchFile) core.BackgroundTask {
	t.Helper()
	input := backgroundFileRemovalBatchInput{Version: backgroundFileRemovalBatchVersion, Mode: "delete", Files: []backgroundFileRemovalBatchFile{file}}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return core.BackgroundTask{
		Kind:        backgroundFileRemovalCleanupTaskKind,
		SubjectKind: "file_batch",
		SubjectID:   "selection",
		InputKey:    string(encoded),
	}
}
