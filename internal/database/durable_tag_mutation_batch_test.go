package database

import (
	"fmt"
	"testing"
)

func TestCreateBackgroundTagMutationSnapshotBatchesFileTargets(t *testing.T) {
	store := newMemoryTestStore(t)
	const operationID = "batch-target-snapshot"
	const columns = 3
	targetCount := maxVars/columns + 17
	targetIDs := make([]string, targetCount)
	for i := range targetIDs {
		targetIDs[i] = fmt.Sprintf("file_%04d", i)
	}

	_, created, err := store.CreateBackgroundOperationWithPendingLimit(operationID, "tag_mutation", false, int64(targetCount), 8)
	if err != nil {
		t.Fatalf("reserve background operation: %v", err)
	}
	if !created {
		t.Fatal("background operation reservation was not created")
	}

	matched, err := store.CreateBackgroundTagMutationSnapshot(
		operationID,
		"add",
		[]byte(`{"file_ids":["snapshot"]}`),
		[]byte(`["tag"]`),
		BackgroundTagTargetFileID,
		targetIDs,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("create background tag mutation snapshot: %v", err)
	}
	if matched != targetCount {
		t.Fatalf("matched targets = %d, want %d", matched, targetCount)
	}

	got, err := store.ListBackgroundTagMutationTargets(operationID)
	if err != nil {
		t.Fatalf("list background tag mutation targets: %v", err)
	}
	if len(got) != len(targetIDs) {
		t.Fatalf("listed targets = %d, want %d", len(got), len(targetIDs))
	}
	for i := range targetIDs {
		if got[i] != targetIDs[i] {
			t.Fatalf("target %d = %q, want %q", i, got[i], targetIDs[i])
		}
	}
}
