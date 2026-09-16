package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestSegmentedUploadImportsPersistOnlyChildTaskState(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	operation, tasks, err := client.CreateBackgroundOperationWithTasks(
		core.BackgroundOperationRequest{Kind: backgroundUploadImportOperationKind, Visible: true, ProgressTotal: 2},
		[]core.BackgroundTaskRequest{
			{Kind: backgroundUploadTaskKind, SubjectKind: "upload_segment", SubjectID: "0"},
			{Kind: backgroundUploadTaskKind, SubjectKind: "upload_segment", SubjectID: "1"},
		},
	)
	if err != nil {
		t.Fatalf("create segmented operation: %v", err)
	}

	library := NewGooruLibrary(client, false)
	for index, task := range tasks {
		path := filepath.Join(root, fmt.Sprintf("segment-%d.txt", index))
		content := []byte{byte('a' + index), byte('0' + index)}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write staged segment %d: %v", index, err)
		}
		response, err := library.importUploadedFiles(context.Background(), []StagedUpload{{
			Name:         filepath.Base(path),
			Path:         path,
			AnalysisPath: path,
			Size:         int64(len(content)),
			TargetID:     "default",
		}}, []string{"source:test"}, backgroundUploadImportState{taskID: task.ID})
		if err != nil {
			t.Fatalf("import segment %d: %v", index, err)
		}
		if response.AffectedCount != 1 {
			t.Fatalf("segment %d affected count = %d, want 1", index, response.AffectedCount)
		}
		var checkpoint backgroundUploadCheckpoint
		found, err := client.GetBackgroundTaskCheckpoint(task.ID, &checkpoint)
		if err != nil || !found {
			t.Fatalf("segment %d task checkpoint = found %v err %v", index, found, err)
		}
		if checkpoint.Phase != backgroundUploadPhaseImported || checkpoint.Response == nil || checkpoint.Response.AffectedCount != 1 {
			t.Fatalf("segment %d checkpoint = %+v, want imported response", index, checkpoint)
		}
	}

	var operationCheckpoint backgroundUploadCheckpoint
	found, err := client.GetBackgroundOperationCheckpoint(operation.ID, &operationCheckpoint)
	if err != nil {
		t.Fatalf("load operation checkpoint: %v", err)
	}
	if found {
		t.Fatalf("segmented child imports overwrote shared operation checkpoint: %+v", operationCheckpoint)
	}
}
