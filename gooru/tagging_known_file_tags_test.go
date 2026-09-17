package gooru

import (
	"errors"
	"testing"

	"gooru.local/types"
)

func TestTagKnownFilesByHashTagsRollsBackExistingAndNewContentTogether(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	client.SetFileRegistrationHooks()
	existing := types.LocationInfo{Path: "/library/existing.jpg", Hash: "hash-atomic-existing", Size: 11, ModTime: 21, Extension: ".jpg"}
	if _, err := client.TagKnownFilesWithBackgroundTasksByHashTags([]types.LocationInfo{existing}, map[string][]string{existing.Hash: {"seed"}}, nil, nil); err != nil {
		t.Fatalf("seed existing content: %v", err)
	}
	client.ResetFileRegistrationHooks()

	fresh := types.LocationInfo{Path: "/library/fresh.jpg", Hash: "hash-atomic-fresh", Size: 12, ModTime: 22, Extension: ".jpg"}
	_, err := client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(
		[]types.LocationInfo{fresh},
		map[string][]string{existing.Hash: {"duplicate:tag"}, fresh.Hash: {"fresh:tag"}},
		nil,
		func(int) (BackgroundOperationTransactionState, error) {
			return BackgroundOperationTransactionState{}, errors.New("stop before commit")
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected state builder failure")
	}
	tags, err := client.store.GetTagsForContent(existing.Hash)
	if err != nil {
		t.Fatalf("GetTagsForContent(existing): %v", err)
	}
	if !sameStringSet(tags, []string{"seed"}) {
		t.Fatalf("existing tags committed despite rollback: %#v", tags)
	}
	exists, err := client.ContentExists(fresh.Hash)
	if err != nil {
		t.Fatalf("ContentExists(fresh): %v", err)
	}
	if exists {
		t.Fatal("fresh content committed despite rollback")
	}
	operations, err := client.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("ListBackgroundOperations: %v", err)
	}
	if len(operations) != 0 {
		t.Fatalf("registration follow-up committed despite rollback: %+v", operations)
	}
}

func TestTagKnownFilesByHashTagsSchedulesAggregatedRegistrationSweep(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	changes, unsubscribe := client.SubscribeBackgroundOperationChanges()
	defer unsubscribe()

	first := types.LocationInfo{Path: "/library/first.jpg", Hash: "hash-known-first", Size: 11, ModTime: 21, Extension: ".jpg"}
	if _, err := client.TagKnownFilesWithBackgroundTasksByHashTags([]types.LocationInfo{first}, nil, nil, nil); err != nil {
		t.Fatalf("register first known file: %v", err)
	}
	select {
	case <-changes:
	default:
		t.Fatal("known-file registration did not signal committed background operation change")
	}

	operations, err := client.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("ListBackgroundOperations(first): %v", err)
	}
	if len(operations) != 1 {
		t.Fatalf("visible operation count = %d, want one registration sweep: %+v", len(operations), operations)
	}
	operation := operations[0]
	if operation.Kind != BackgroundMediaMetadataSweepOperationKind || operation.Status != BackgroundWorkPending {
		t.Fatalf("unexpected registration sweep: %+v", operation)
	}
	task, found, err := client.GetBackgroundOperationTask(operation.ID)
	if err != nil {
		t.Fatalf("GetBackgroundOperationTask: %v", err)
	}
	if !found || task.Kind != BackgroundMediaMetadataSweepTaskKind {
		t.Fatalf("unexpected registration sweep task: found=%v task=%+v", found, task)
	}

	second := types.LocationInfo{Path: "/library/second.jpg", Hash: "hash-known-second", Size: 12, ModTime: 22, Extension: ".jpg"}
	if _, err := client.TagKnownFilesWithBackgroundTasksByHashTags([]types.LocationInfo{second}, nil, nil, nil); err != nil {
		t.Fatalf("register second known file: %v", err)
	}
	operations, err = client.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("ListBackgroundOperations(second): %v", err)
	}
	if len(operations) != 1 || operations[0].ID != operation.ID {
		t.Fatalf("registration sweeps were not coalesced: %+v", operations)
	}
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(got))
	for _, value := range got {
		counts[value]++
	}
	for _, value := range want {
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}
