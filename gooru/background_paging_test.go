package gooru

import "testing"

func TestListBackgroundOperationsSupportsOffsetAndVisibleCount(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	visibleOld, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-old", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "hidden", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	visibleNew, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-new", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET created_at = ? WHERE id = ?`, int64(100), visibleOld.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET created_at = ? WHERE id = ?`, int64(200), hidden.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET created_at = ? WHERE id = ?`, int64(300), visibleNew.ID); err != nil {
		t.Fatal(err)
	}

	page, err := client.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list second visible page: %v", err)
	}
	if len(page) != 1 || page[0].ID != visibleOld.ID {
		t.Fatalf("second visible page = %+v", page)
	}

	visibleCount, err := client.CountBackgroundOperations(true)
	if err != nil {
		t.Fatalf("count visible operations: %v", err)
	}
	if visibleCount != 2 {
		t.Fatalf("visible count = %d, want 2", visibleCount)
	}
	allCount, err := client.CountBackgroundOperations(false)
	if err != nil {
		t.Fatalf("count all operations: %v", err)
	}
	if allCount != 3 {
		t.Fatalf("all count = %d, want 3", allCount)
	}
}
