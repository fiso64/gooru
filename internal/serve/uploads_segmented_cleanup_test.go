package serve

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestSegmentedCanceledUploadCleanupUsesTaskCheckpointsAndSkipsMissingSegments(t *testing.T) {
	client, operationID := newSegmentedCleanupTestOperation(t, 3)
	defer client.Close()

	attachSegmentedCleanupTestTask(t, client, operationID, 0, backgroundUploadInitialCheckpoint())
	attachSegmentedCleanupTestTask(t, client, operationID, 2, backgroundUploadInitialCheckpoint())

	if result, err := client.CancelBackgroundOperationWithCleanupTask(operationID, backgroundUploadCleanupTaskRequest(operationID)); err != nil || !result.Canceled {
		t.Fatalf("cancel segmented upload = %+v, %v", result, err)
	}

	server := NewServer(DefaultConfig(""))
	handler := server.backgroundUploadCleanupHandlerV2(client)
	if err := handler(context.Background(), segmentedCleanupTestTask(operationID)); err != nil {
		t.Fatalf("cleanup segmented upload: %v", err)
	}

	for _, segmentIndex := range []int64{0, 2} {
		task, found, err := client.GetBackgroundTask(durableUploadSegmentTaskID(operationID, segmentIndex))
		if err != nil || !found {
			t.Fatalf("load segment %d after cancellation = found %v err %v", segmentIndex, found, err)
		}
		if task.Status != core.BackgroundWorkCanceled {
			t.Fatalf("segment %d status = %q, want canceled", segmentIndex, task.Status)
		}
	}
}

func TestSegmentedCanceledUploadCleanupProcessesLaterChild(t *testing.T) {
	client, operationID := newSegmentedCleanupTestOperation(t, 3)
	defer client.Close()

	attachSegmentedCleanupTestTask(t, client, operationID, 0, backgroundUploadInitialCheckpoint())
	attachSegmentedCleanupTestTask(t, client, operationID, 2, backgroundUploadCheckpoint{Phase: "invalid-segment-phase"})

	if result, err := client.CancelBackgroundOperationWithCleanupTask(operationID, backgroundUploadCleanupTaskRequest(operationID)); err != nil || !result.Canceled {
		t.Fatalf("cancel segmented upload = %+v, %v", result, err)
	}

	server := NewServer(DefaultConfig(""))
	err := server.backgroundUploadCleanupHandlerV2(client)(context.Background(), segmentedCleanupTestTask(operationID))
	if err == nil {
		t.Fatal("segmented cleanup unexpectedly ignored invalid later child checkpoint")
	}
	if !strings.Contains(err.Error(), "segment 2") || !strings.Contains(err.Error(), "invalid-segment-phase") {
		t.Fatalf("segmented cleanup error = %v, want later child checkpoint failure", err)
	}
}

func TestSegmentedTerminalFailureCleanupOnlyProcessesFailedChild(t *testing.T) {
	client, operationID := newSegmentedCleanupTestOperation(t, 2)
	defer client.Close()

	attachSegmentedCleanupTestTask(t, client, operationID, 0, backgroundUploadInitialCheckpoint())
	attachSegmentedCleanupTestTask(t, client, operationID, 1, backgroundUploadCheckpoint{Phase: "invalid-sibling-phase"})

	server := NewServer(DefaultConfig(""))
	sourceTaskID := durableUploadSegmentTaskID(operationID, 0)
	task := segmentedCleanupTestTask(operationID)
	task.ID = sourceTaskID + ":terminal-cleanup"
	if err := server.backgroundUploadCleanupHandlerV2(client)(context.Background(), task); err != nil {
		t.Fatalf("cleanup failed segment: %v", err)
	}

	sibling, found, err := client.GetBackgroundTask(durableUploadSegmentTaskID(operationID, 1))
	if err != nil || !found {
		t.Fatalf("load sibling after failed-child cleanup = found %v err %v", found, err)
	}
	if sibling.Status != core.BackgroundWorkPending {
		t.Fatalf("sibling status = %q, want pending", sibling.Status)
	}
}

func newSegmentedCleanupTestOperation(t *testing.T, segmentCount int64) (*core.Client, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	operation, err := client.CreateBackgroundOperation(core.BackgroundOperationRequest{
		Kind:          backgroundUploadImportOperationKind,
		Visible:       true,
		ProgressTotal: segmentCount,
	})
	if err != nil {
		client.Close()
		t.Fatalf("create upload operation: %v", err)
	}
	return client, operation.ID
}

func attachSegmentedCleanupTestTask(t *testing.T, client *core.Client, operationID string, segmentIndex int64, checkpoint backgroundUploadCheckpoint) {
	t.Helper()
	request, err := backgroundUploadTaskRequest(operationID, []savedUpload{{
		name:     "skipped.jpg",
		targetID: "default",
		status:   "skipped",
	}}, nil)
	if err != nil {
		t.Fatalf("build segment %d task: %v", segmentIndex, err)
	}
	if _, created, err := client.AttachBackgroundTaskToOperation(operationID, durableUploadSegmentTaskID(operationID, segmentIndex), checkpoint, request); err != nil || !created {
		t.Fatalf("attach segment %d = created %v err %v", segmentIndex, created, err)
	}
}

func segmentedCleanupTestTask(operationID string) core.BackgroundTask {
	return core.BackgroundTask{
		Kind:        backgroundUploadCleanupTaskKind,
		SubjectKind: "operation",
		SubjectID:   operationID,
	}
}
