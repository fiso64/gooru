package gooru

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gooru.local/types"
)

func TestDefaultRegistrationMetadataSweepIsDurableAndAggregatedAcrossClients(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	observer, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open observer client: %v", err)
	}
	t.Cleanup(func() { _ = observer.Close() })

	register := func(name, contents string) {
		t.Helper()
		producer, err := New(dbPath, false)
		if err != nil {
			t.Fatalf("open producer client: %v", err)
		}
		mediaPath := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(mediaPath, []byte(contents), 0o600); err != nil {
			_ = producer.Close()
			t.Fatalf("write media file: %v", err)
		}
		if _, err := producer.TagFiles([]string{mediaPath}, []string{"external"}, nil, false); err != nil {
			_ = producer.Close()
			t.Fatalf("tag external file: %v", err)
		}
		if err := producer.Close(); err != nil {
			t.Fatalf("close producer client: %v", err)
		}
	}

	register("external-a.jpg", "independent client registration a")
	register("external-b.jpg", "independent client registration b")

	operations, err := observer.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(operations) != 1 {
		t.Fatalf("visible operation count = %d, want one aggregated sweep: %+v", len(operations), operations)
	}
	operation := operations[0]
	if operation.Kind != BackgroundMediaMetadataSweepOperationKind || operation.Status != BackgroundWorkPending || operation.ProgressTotal != 0 {
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

func TestDefaultRegistrationMetadataSweepSignalsCommittedOperationChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	producer, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open producer client: %v", err)
	}
	t.Cleanup(func() { _ = producer.Close() })
	changes, unsubscribe := producer.SubscribeBackgroundOperationChanges()
	defer unsubscribe()

	mediaPath := filepath.Join(t.TempDir(), "local-registration.jpg")
	if err := os.WriteFile(mediaPath, []byte("same process registration"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	if _, err := producer.TagFiles([]string{mediaPath}, []string{"local"}, nil, false); err != nil {
		t.Fatalf("tag local file: %v", err)
	}

	select {
	case <-changes:
	default:
		t.Fatal("committed registration metadata sweep did not signal operation change")
	}
	operations, err := producer.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(operations) != 1 || operations[0].Kind != BackgroundMediaMetadataSweepOperationKind || operations[0].Status != BackgroundWorkPending {
		t.Fatalf("unexpected committed metadata operation: %+v", operations)
	}
}

func TestRunMediaMetadataSweepCreatesObservableNoWorkJob(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	created, err := client.RunMediaMetadataSweep()
	if err != nil {
		t.Fatalf("run metadata sweep with no pending work: %v", err)
	}
	if !created {
		t.Fatal("manual metadata sweep did not create an observable no-work job")
	}
	running, err := client.MediaMetadataSweepRunning()
	if err != nil {
		t.Fatalf("inspect running metadata sweep: %v", err)
	}
	if !running {
		t.Fatal("manual metadata sweep was not reported as running")
	}
	created, err = client.RunMediaMetadataSweep()
	if err != nil {
		t.Fatalf("run metadata sweep while active: %v", err)
	}
	if created {
		t.Fatal("manual metadata sweep stacked a duplicate active operation")
	}

	operations, err := client.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(operations) != 1 || operations[0].Kind != BackgroundMediaMetadataSweepOperationKind || operations[0].Status != BackgroundWorkPending {
		t.Fatalf("unexpected manual metadata operation: %+v", operations)
	}
}

func TestRunMediaMetadataSweepIsAtomicAcrossClients(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const clientCount = 8
	clients := make([]*Client, 0, clientCount)
	for i := 0; i < clientCount; i++ {
		client, err := New(dbPath, false)
		if err != nil {
			t.Fatalf("open client %d: %v", i, err)
		}
		clients = append(clients, client)
	}
	t.Cleanup(func() {
		for _, client := range clients {
			_ = client.Close()
		}
	})

	start := make(chan struct{})
	created := make(chan bool, clientCount)
	errs := make(chan error, clientCount)
	var wg sync.WaitGroup
	for _, client := range clients {
		client := client
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			didCreate, err := client.RunMediaMetadataSweep()
			if err != nil {
				errs <- err
				return
			}
			created <- didCreate
		}()
	}
	close(start)
	wg.Wait()
	close(created)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent manual metadata sweep: %v", err)
	}
	createdCount := 0
	for didCreate := range created {
		if didCreate {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created metadata sweep count = %d, want 1", createdCount)
	}

	operations, err := clients[0].ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(operations) != 1 || operations[0].Kind != BackgroundMediaMetadataSweepOperationKind || operations[0].Status != BackgroundWorkPending {
		t.Fatalf("unexpected concurrent manual metadata operations: %+v", operations)
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
	if len(operations) != 1 || operations[0].Kind != BackgroundMediaMetadataSweepOperationKind || operations[0].Status != BackgroundWorkPending || operations[0].ProgressTotal != 0 {
		t.Fatalf("unexpected recovery operations: %+v", operations)
	}
}
