package serve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/types"
)

func TestRemoveCanceledSavedUploadsReportsRetriableFailureWithoutPath(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, "blocked-stage")
	if err := os.Mkdir(stagedPath, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(stagedPath, "still-present")
	if err := os.WriteFile(child, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := removeCanceledSavedUploads([]savedUpload{{path: stagedPath}})
	if err == nil {
		t.Fatal("cleanup unexpectedly succeeded while staged path was non-empty")
	}
	if strings.Contains(err.Error(), stagedPath) {
		t.Fatalf("cleanup error leaked staged path: %v", err)
	}
	if _, statErr := os.Stat(stagedPath); statErr != nil {
		t.Fatalf("failed cleanup removed staged path unexpectedly: %v", statErr)
	}

	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := removeCanceledSavedUploads([]savedUpload{{path: stagedPath}}); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("retry cleanup left staged path behind: %v", err)
	}
}

func TestCanceledUploadCleanupFailureRetriesThroughDurableRunner(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	stagedPath := filepath.Join(root, "blocked-stage")
	if err := os.Mkdir(stagedPath, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(stagedPath, "still-present")
	if err := os.WriteFile(child, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	operation, err := client.CreateBackgroundOperation(core.BackgroundOperationRequest{
		Kind:          backgroundUploadImportOperationKind,
		Visible:       false,
		ProgressTotal: 1,
	})
	if err != nil {
		t.Fatalf("create upload operation: %v", err)
	}
	taskRequest, err := backgroundUploadTaskRequest(operation.ID, []savedUpload{{
		name: "blocked-stage", path: stagedPath, destinationPath: stagedPath, size: 1, targetID: "default",
	}}, nil)
	if err != nil {
		t.Fatalf("build upload task: %v", err)
	}
	if _, err := client.AttachBackgroundTaskAndRevealOperation(operation.ID, backgroundUploadInitialCheckpoint(), taskRequest); err != nil {
		t.Fatalf("attach upload task: %v", err)
	}
	if result, err := client.CancelBackgroundOperationWithCleanupTask(operation.ID, backgroundUploadCleanupTaskRequest(operation.ID)); err != nil || !result.Canceled {
		t.Fatalf("cancel upload with cleanup = %+v, %v", result, err)
	}

	server := NewServer(DefaultConfig(dbPath))
	runtime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
		ResourceClass: backgroundUploadResourceClass,
		WorkerID:      "upload-cleanup-retry",
		Handlers: map[string]core.BackgroundTaskHandler{
			backgroundUploadCleanupTaskKind: server.backgroundUploadCleanupHandler(client),
		},
		LeaseDuration: 300 * time.Millisecond,
		PollInterval:  10 * time.Millisecond,
		RetryDelay:    50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create cleanup runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case runErr := <-done:
			if runErr != nil {
				t.Errorf("cleanup runtime: %v", runErr)
			}
		case <-time.After(time.Second):
			t.Error("cleanup runtime did not stop")
		}
	}()

	inspection, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("open inspection store: %v", err)
	}
	defer inspection.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		var status string
		var attempts int
		err := inspection.DB.QueryRow(`
			SELECT status, attempt_count FROM background_tasks
			WHERE dedupe_key = ?
		`, "upload-cleanup:"+operation.ID).Scan(&status, &attempts)
		if err == nil && status == "pending" && attempts >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup failure was not durably requeued: status=%q attempts=%d err=%v", status, attempts, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		var status string
		var attempts int
		err := inspection.DB.QueryRow(`
			SELECT status, attempt_count FROM background_tasks
			WHERE dedupe_key = ?
		`, "upload-cleanup:"+operation.ID).Scan(&status, &attempts)
		if err == nil && status == "completed" && attempts >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup retry did not complete: status=%q attempts=%d err=%v", status, attempts, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("completed cleanup retry left staging behind: %v", err)
	}
}
