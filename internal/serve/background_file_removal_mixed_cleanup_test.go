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

func TestBackgroundFileRemovalCleanupHandlerReconcilesMixedBatch(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "managed.jpg")
	stagingPath := filepath.Join(root, ".gooru-delete-mixed-000000")
	if err := os.WriteFile(stagingPath, []byte("managed-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newCanceledRemovalTestServer(root, map[string]types.FileInfo{
		"managed": {PublicID: "managed", Path: originalPath},
	})
	input := backgroundFileRemovalBatchInput{
		Version: backgroundFileRemovalMixedBatchVersion,
		Mode:    "delete_or_untrack",
		Files: []backgroundFileRemovalBatchFile{
			{PublicID: "managed", OriginalPath: originalPath, StagingPath: stagingPath},
			{PublicID: "external"},
		},
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	task := core.BackgroundTask{
		Kind:        backgroundFileRemovalCleanupTaskKind,
		SubjectKind: "file_batch",
		SubjectID:   "selection",
		InputKey:    string(encoded),
	}

	if err := server.backgroundFileRemovalCleanupHandler(nil)(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, originalPath, "managed-original")
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mixed cleanup staging path still exists after restore: %v", err)
	}
}
