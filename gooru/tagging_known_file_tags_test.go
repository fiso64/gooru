package gooru

import (
	"errors"
	"testing"

	"gooru.local/types"
)

func TestTagKnownFilesByHashTagsRollsBackExistingAndNewContentTogether(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	existing := types.LocationInfo{Path: "/library/existing.jpg", Hash: "hash-atomic-existing", Size: 11, ModTime: 21, Extension: ".jpg"}
	if _, err := client.TagKnownFilesWithBackgroundTasksByHashTags([]types.LocationInfo{existing}, map[string][]string{existing.Hash: {"seed"}}, nil, nil); err != nil {
		t.Fatalf("seed existing content: %v", err)
	}
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
