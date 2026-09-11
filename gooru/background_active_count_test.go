package gooru

import "testing"

func TestCountActiveBackgroundOperationsIsExactAndCanFilterHidden(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)

	visiblePending, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-pending", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	visibleRunning, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-running", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	hiddenPending, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "hidden-pending", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	visibleCompleted, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-completed", Visible: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.store.DB.Exec(`UPDATE background_operations SET status = 'running' WHERE id = ?`, visibleRunning.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET status = 'completed' WHERE id = ?`, visibleCompleted.ID); err != nil {
		t.Fatal(err)
	}

	visibleCount, err := client.CountActiveBackgroundOperations(true)
	if err != nil {
		t.Fatalf("count visible active operations: %v", err)
	}
	if visibleCount != 2 {
		t.Fatalf("visible active count = %d, want 2", visibleCount)
	}

	allCount, err := client.CountActiveBackgroundOperations(false)
	if err != nil {
		t.Fatalf("count all active operations: %v", err)
	}
	if allCount != 3 {
		t.Fatalf("all active count = %d, want 3", allCount)
	}

	_ = visiblePending
	_ = hiddenPending
}
