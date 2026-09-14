package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestDefaultRegistrationMetadataSweepIsDurableAcrossClients(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	observer, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open observer client: %v", err)
	}
	t.Cleanup(func() { _ = observer.Close() })
	producer, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open producer client: %v", err)
	}

	mediaPath := filepath.Join(t.TempDir(), "external.jpg")
	if err := os.WriteFile(mediaPath, []byte("independent client registration"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	if _, err := producer.TagFiles([]string{mediaPath}, []string{"external"}, nil, false); err != nil {
		t.Fatalf("tag external file: %v", err)
	}
	if err := producer.Close(); err != nil {
		t.Fatalf("close producer client: %v", err)
	}

	operations, err := observer.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(operations) != 1 {
		t.Fatalf("visible operation count = %d, want 1: %+v", len(operations), operations)
	}
	operation := operations[0]
	if operation.Kind != BackgroundMediaMetadataSweepOperationKind || operation.Status != BackgroundWorkPending || operation.ProgressTotal != 1 {
		t.Fatalf("unexpected metadata operation: %+v", operation)
	}
	task, found, err := observer.GetBackgroundOperationTask(operation.ID)
	if err != nil {
		t.Fatalf("get metadata operation task: %v", err)
	}
	if !found {
		t.Fatal("metadata operation task not found")
	}
	if task.OperationID != operation.ID || task.Kind != BackgroundMediaMetadataSweepTaskKind || task.SubjectKind != "library" || task.SubjectID != "media-metadata" {
		t.Fatalf("unexpected metadata task: %+v", task)
	}
}

func TestEnsureMediaMetadataSweepRecoversMissingWakeWithoutDuplicates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	producer, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open producer client: %v", err)
	}
	producer.SetFileRegistrationHooks(func(FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		return nil, nil
	})

	mediaPath := filepath.Join(t.TempDir(), "missed.jpg")
	if err := os.WriteFile(mediaPath, []byte("registration from buggy producer"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	if _, err := producer.TagFiles([]string{mediaPath}, []string{"external"}, nil, false); err != nil {
		t.Fatalf("register file without wake: %v", err)
	}
	if err := producer.Close(); err != nil {
		t.Fatalf("close producer client: %v", err)
	}

	recovery, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open recovery client: %v", err)
	}
	created, err := recovery.EnsureMediaMetadataSweep()
	if err != nil {
		t.Fatalf("ensure metadata sweep: %v", err)
	}
	if !created {
		t.Fatal("metadata recovery sweep was not created")
	}
	created, err = recovery.EnsureMediaMetadataSweep()
	if err != nil {
		t.Fatalf("ensure metadata sweep again: %v", err)
	}
	if created {
		t.Fatal("duplicate metadata recovery sweep was created")
	}
	if err := recovery.Close(); err != nil {
		t.Fatalf("close recovery client: %v", err)
	}

	restarted, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open restarted client: %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	created, err = restarted.EnsureMediaMetadataSweep()
	if err != nil {
		t.Fatalf("ensure metadata sweep after restart: %v", err)
	}
	if created {
		t.Fatal("restart stacked a duplicate metadata recovery sweep")
	}
	operations, err := restarted.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(operations) != 1 || operations[0].Kind != BackgroundMediaMetadataSweepOperationKind || operations[0].Status != BackgroundWorkPending {
		t.Fatalf("unexpected recovery operations: %+v", operations)
	}
}
