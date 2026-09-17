package database

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
)

func TestCreateBackgroundTagMutationSnapshotBatchesFileTargets(t *testing.T) {
	store := newMemoryTestStore(t)
	var queryLog bytes.Buffer
	store.logger = log.New(&queryLog, "", 0)

	const operationID = "batch-target-snapshot"
	const columns = 3
	targetCount := maxVars/columns + 17
	targetIDs := make([]string, targetCount)
	for i := range targetIDs {
		targetIDs[i] = fmt.Sprintf("file_%04d", i)
	}
	// Repeat an earlier target in the second batch so INSERT OR IGNORE must
	// report only the unique target cardinality across the batch boundary.
	targetIDs[len(targetIDs)-1] = targetIDs[0]
	uniqueTargetCount := targetCount - 1

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
	if matched != uniqueTargetCount {
		t.Fatalf("matched targets = %d, want %d", matched, uniqueTargetCount)
	}

	logged := queryLog.String()
	const insertPrefix = "INSERT OR IGNORE INTO background_tag_mutation_targets"
	if got := strings.Count(logged, insertPrefix); got != 2 {
		t.Fatalf("target insert statements = %d, want 2 across the bind-budget boundary; log:\n%s", got, logged)
	}
	if !strings.Contains(logged, "-- ARGS: 900 bound values redacted") {
		t.Fatalf("first target batch did not use the expected 900-bind budget; log:\n%s", logged)
	}
	if strings.Contains(logged, "-- ARGS: 901 bound values redacted") {
		t.Fatalf("target snapshot exceeded maxVars; log:\n%s", logged)
	}
	if strings.Contains(logged, "SELECT COUNT(*)\n\t\tFROM background_tag_mutation_targets") {
		t.Fatalf("target snapshot issued a redundant target-count query; log:\n%s", logged)
	}

	got, err := store.ListBackgroundTagMutationTargets(operationID)
	if err != nil {
		t.Fatalf("list background tag mutation targets: %v", err)
	}
	if len(got) != uniqueTargetCount {
		t.Fatalf("listed targets = %d, want %d", len(got), uniqueTargetCount)
	}
	for i := 0; i < uniqueTargetCount; i++ {
		if got[i] != targetIDs[i] {
			t.Fatalf("target %d = %q, want %q", i, got[i], targetIDs[i])
		}
	}
}
