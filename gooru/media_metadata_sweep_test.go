package gooru

import "testing"

func TestMediaMetadataSweepContinuationCursorRoundTrips(t *testing.T) {
	request, err := mediaMetadataSweepContinuationTaskRequest("operation-test", 123)
	if err != nil {
		t.Fatal(err)
	}
	if request.OperationID != "operation-test" || request.Kind != BackgroundMediaMetadataSweepTaskKind || request.SubjectKind != "library" || request.SubjectID != "media-metadata" || request.ResourceClass != BackgroundMediaMetadataResourceClass || request.MaxAttempts != 5 {
		t.Fatalf("unexpected continuation request: %#v", request)
	}

	afterLocationID, err := MediaMetadataSweepAfterLocationID(BackgroundTask{InputKey: request.InputKey})
	if err != nil {
		t.Fatal(err)
	}
	if afterLocationID != 123 {
		t.Fatalf("continuation cursor = %d, want 123", afterLocationID)
	}

	afterLocationID, err = MediaMetadataSweepAfterLocationID(BackgroundTask{InputKey: "media-metadata-wake-existing"})
	if err != nil {
		t.Fatal(err)
	}
	if afterLocationID != 0 {
		t.Fatalf("initial wake cursor = %d, want 0", afterLocationID)
	}

	if _, err := MediaMetadataSweepAfterLocationID(BackgroundTask{InputKey: mediaMetadataSweepCursorPrefix + "bad"}); err == nil {
		t.Fatal("malformed continuation cursor was accepted")
	}
}

func TestMediaMetadataSweepContinuationUsesActiveDedupe(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, err := client.CreateBackgroundOperation(BackgroundOperationRequest{
		Kind:    BackgroundMediaMetadataSweepOperationKind,
		Visible: true,
	})
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}

	created, err := client.EnqueueMediaMetadataSweepContinuation(operation.ID, 64)
	if err != nil {
		t.Fatalf("enqueue continuation: %v", err)
	}
	if !created {
		t.Fatal("first continuation enqueue reported existing work")
	}
	created, err = client.EnqueueMediaMetadataSweepContinuation(operation.ID, 64)
	if err != nil {
		t.Fatalf("dedupe continuation: %v", err)
	}
	if created {
		t.Fatal("duplicate active continuation was created")
	}

	var count int
	var inputKey string
	if err := client.store.DB.QueryRow(`
		SELECT count(*), min(input_key)
		FROM background_tasks
		WHERE operation_id = ? AND kind = ?
	`, operation.ID, BackgroundMediaMetadataSweepTaskKind).Scan(&count, &inputKey); err != nil {
		t.Fatalf("inspect continuation: %v", err)
	}
	if count != 1 {
		t.Fatalf("continuation task count = %d, want 1", count)
	}
	if inputKey != mediaMetadataSweepCursorPrefix+"64" {
		t.Fatalf("continuation input key = %q, want %q", inputKey, mediaMetadataSweepCursorPrefix+"64")
	}
}

func TestMediaMetadataSweepContinuationRejectsInvalidIdentity(t *testing.T) {
	if _, err := mediaMetadataSweepContinuationTaskRequest("", 1); err == nil {
		t.Fatal("continuation without operation id was accepted")
	}
	if _, err := mediaMetadataSweepContinuationTaskRequest("operation-test", 0); err == nil {
		t.Fatal("continuation with zero cursor was accepted")
	}
}
